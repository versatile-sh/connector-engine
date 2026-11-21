package storetest

import (
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"slices"
	"strings"

	"github.com/versatile-sh/connector-engine/port"
)

const restoreSecret = "do-not-echo-this-reason"

func seedRestore(e *env) port.Snapshotter {
	src, ok := e.s.(port.Snapshotter)
	if !ok {
		e.t.Skip("the store takes no snapshot")
	}
	e.must(e.write(inst, func(tx port.Tx) error {
		return errors.Join(tx.PutTenant(port.Tenant{ID: "ta", State: port.TenantActive, CreatedAt: t0}),
			tx.PutListing(port.Listing{Ref: port.ListingRef{Name: "io.x/svc"}, Source: "own", Publisher: "x", Status: port.ListingActive}))
	}))
	e.must(e.write(scopeA, func(tx port.Tx) error {
		_, err := tx.PutConnector(connector("ta", "c1"), port.AnyRevision)
		g := agentGrant("ta", "g1", "*", 0)
		g.Reason = restoreSecret
		dangling := agentGrant("ta", "g2", "fn:gone", 0)
		dangling.ScopeKind, dangling.ScopeID, dangling.Connector, dangling.DefinitionHash = port.ScopeUser, "alice", "c-gone", "h"
		gone := agentGrant("ta", "g3", "fn:gone", 0)
		gone.DefinitionHash = "h"
		return errors.Join(err, tx.PutGrant(g), tx.PutGrant(dangling), tx.PutGrant(gone),
			tx.PutBinding(port.Binding{Tenant: "ta", Workspace: "w", Connector: "c1", Agent: "x", CredentialPolicy: port.CredentialPolicy("shared")}))
	}))
	return src
}

type rowEdit func(snap *port.Snapshot, table string, i int, row, doc map[string]any)

func editDoc(set map[string]any) rowEdit {
	return func(snap *port.Snapshot, table string, i int, row, doc map[string]any) {
		maps.Copy(doc, set)
		snap.Tables[table][i] = marshalRow(row, doc)
	}
}

func marshalRow(row, doc map[string]any) json.RawMessage {
	if _, ok := row["v"]; ok {
		row["v"] = doc
	} else {
		row["doc"] = doc
	}
	raw, _ := json.Marshal(row)
	return raw
}

func cloneSnapshot(snap port.Snapshot) port.Snapshot {
	out := port.Snapshot{Store: snap.Store, Tables: map[string][]json.RawMessage{}}
	for name, rows := range snap.Tables {
		out.Tables[name] = slices.Clone(rows)
	}
	return out
}

func edited(e *env, snap port.Snapshot, match func(doc map[string]any) bool, edit rowEdit) port.Snapshot {
	e.t.Helper()
	out := cloneSnapshot(snap)
	for table, rows := range out.Tables {
		for i, raw := range rows {
			var row map[string]any
			if json.Unmarshal(raw, &row) != nil {
				continue
			}
			doc, _ := row["v"].(map[string]any)
			if doc == nil {
				doc, _ = row["doc"].(map[string]any)
			}
			if doc != nil && match(doc) {
				edit(&out, table, i, row, doc)
				return out
			}
		}
	}
	e.t.Fatal("no row of the snapshot matches")
	return out
}

func isGrant(id string) func(map[string]any) bool {
	return func(doc map[string]any) bool { _, ok := doc["scope_kind"]; return ok && doc["id"] == id }
}

func isBinding(doc map[string]any) bool {
	_, ok := doc["credential_policy"]
	return ok
}

func restoreRefusesBrokenSnapshots(e *env) {
	src := seedRestore(e)
	snap, err := src.Snapshot(ctx)
	e.must(err)
	twin := func(id string) rowEdit {
		return func(snap *port.Snapshot, table string, i int, row, doc map[string]any) {
			copyRow, copyDoc := maps.Clone(row), maps.Clone(doc)
			if id != "" {
				copyDoc["id"] = id
				for _, k := range []string{"k", "K"} {
					if _, ok := copyRow[k]; ok {
						copyRow[k] = id
					}
				}
			}
			snap.Tables[table] = append(snap.Tables[table], marshalRow(copyRow, copyDoc))
		}
	}
	installation := func(snap *port.Snapshot, table string, i int, row, doc map[string]any) {
		doc["tenant"] = ""
		for _, k := range []string{"tenant", "tenant_id"} {
			if _, ok := row[k]; ok {
				row[k] = ""
			}
		}
		snap.Tables[table][i] = marshalRow(row, doc)
	}
	cases := map[string]struct {
		snap port.Snapshot
		name string
	}{
		"unknown mode":                    {edited(e, snap, isGrant("g1"), editDoc(map[string]any{"mode": "sometimes"})), "g1"},
		"unknown selector":                {edited(e, snap, isGrant("g1"), editDoc(map[string]any{"selector": "everything"})), "g1"},
		"session without expiry":          {edited(e, snap, isGrant("g1"), editDoc(map[string]any{"scope_kind": "session", "agent": "x"})), "g1"},
		"another tenant's row":            {edited(e, snap, isGrant("g1"), editDoc(map[string]any{"tenant": "tb"})), "g1"},
		"the same row twice":              {edited(e, snap, isGrant("g1"), twin("")), "g1"},
		"two rows with one key":           {edited(e, snap, isGrant("g1"), twin("g9")), "g9"},
		"a mode of the wrong type":        {edited(e, snap, isGrant("g1"), editDoc(map[string]any{"mode": 5})), ""},
		"a grant of the installation":     {edited(e, snap, isGrant("g1"), installation), "g1"},
		"a binding of the installation":   {edited(e, snap, isBinding, installation), "c1"},
		"a binding without its connector": {edited(e, snap, isBinding, editDoc(map[string]any{"connector": "c-gone"})), "c-gone"},
	}
	future := cloneSnapshot(snap)
	future.Tables["future_rows"] = []json.RawMessage{json.RawMessage(`{"tenant_id":"ta"}`)}
	cases["a table this store does not know"] = struct {
		snap port.Snapshot
		name string
	}{future, "future_rows"}
	before := viewOf(e, e.s, e.o)
	for name, c := range cases {
		_, err := src.Restore(ctx, c.snap, "operator")
		if !port.IsCode(err, port.CodeInvalidArgument) || !strings.Contains(err.Error(), c.name) || strings.Contains(err.Error(), restoreSecret) {
			e.t.Errorf("%s: got %v, want invalid_argument naming %q without the row's values", name, err, c.name)
			continue
		}
		e.same(before, viewOf(e, e.s, e.o))
	}
}

func restoreRecordsItself(e *env) {
	src := seedRestore(e)
	snap, err := src.Snapshot(ctx)
	e.must(err)
	s, o := e.fresh()
	out, err := s.(port.Snapshotter).Restore(ctx, snap, "ops")
	e.must(err)
	e.same(map[string]int{"bindings": 1, "connectors": 1, "functions": 0, "grants": 3, "tenants": 1}, out.Counts)
	slices.Sort(out.Warnings)
	e.same([]any{2, []string{`grant "g2" names connector "c-gone", which the snapshot lacks`, `grant "g3" names function "gone", which the snapshot lacks`}},
		[]any{out.WarningCount, out.Warnings})
	var audit []port.AuditRow
	var events []port.Event
	e.must(s.Read(ctx, inst, func(tx port.Tx) (err error) {
		if audit, err = tx.ListAudit("snapshot"); err == nil {
			events, err = tx.EventsAfter(port.SourceOf(port.Installation), 0, 0)
		}
		return err
	}))
	if len(audit) != 1 || audit[0].Actor != "ops" || audit[0].Action != "connector.snapshot.restored" {
		e.t.Fatalf("audit %+v", audit)
	}
	if len(events) != 1 || events[0].Type != "connector.snapshot.restored" || events[0].Data["by"] != "ops" || events[0].Data["warnings"] != float64(2) {
		e.t.Fatalf("events %+v", events)
	}
	pending, err := o.Pending(ctx, 0)
	e.must(err)
	e.same(1, len(pending))
	future := cloneSnapshot(snap)
	future.Schema = snap.Schema + 1
	if _, err := s.(port.Snapshotter).Restore(ctx, future, "ops"); !port.IsCode(err, port.CodeInvalidArgument) {
		e.t.Fatalf("a snapshot of a newer schema: %v", err)
	}
}

func restoreKeepsTheFirstWarnings(e *env) {
	src := seedRestore(e)
	e.must(e.write(scopeA, func(tx port.Tx) error {
		var err error
		for i := range 30 {
			g := agentGrant("ta", fmt.Sprintf("w%02d", i), fmt.Sprintf("fn:gone%02d", i), 0)
			g.DefinitionHash = "h"
			err = errors.Join(err, tx.PutGrant(g))
		}
		return err
	}))
	snap, err := src.Snapshot(ctx)
	e.must(err)
	s, _ := e.fresh()
	out, err := s.(port.Snapshotter).Restore(ctx, snap, "ops")
	e.must(err)
	e.same([]int{32, 20}, []int{out.WarningCount, len(out.Warnings)})
	var audit []port.AuditRow
	e.must(s.Read(ctx, inst, func(tx port.Tx) (err error) {
		audit, err = tx.ListAudit("snapshot")
		return err
	}))
	if first, _ := audit[0].Detail["first_warnings"].([]any); len(audit) != 1 || len(first) != 20 || audit[0].Detail["warnings"] != float64(32) {
		e.t.Fatalf("audit %+v", audit)
	}
}
