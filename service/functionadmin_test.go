package service_test

import (
	"slices"
	"strings"
	"testing"

	"github.com/versatile-sh/connector-engine/catalog"
	"github.com/versatile-sh/connector-engine/port"
	"github.com/versatile-sh/connector-engine/server"
	st "github.com/versatile-sh/connector-engine/service/servicetest"
)

type functionAct struct {
	prepare func(w *world)
	act     func(w *world)
}

func projectable(w *world) {
	def, _ := st.Definition("search", "Searches", `"outputSchema":{"type":"object","properties":{"title":{"type":"string"}}}`)
	list := w.redefine("search", "")
	for i := range list {
		if list[i].Name == "search" {
			list[i] = def
		}
	}
	w.Runtime.SetDefinitions(listing, list...)
	st.Ok(w.Svc.Relist(w.Ctx, tenant, w.conn.ID))
	w.acceptAll()
}

var functionActs = map[string]functionAct{
	"AcceptFunction": {act: func(w *world) {
		st.Ok(w.Svc.AcceptFunction(w.Ctx, w.admin, w.conn.ID, "search", w.hashes["search"], false))
	}},
	"AcceptFunctions": {act: func(w *world) {
		st.Ok(w.Svc.AcceptFunctions(w.Ctx, w.admin, w.conn.ID, map[string]port.Hash{"search": w.hashes["search"]}))
	}},
	"AcceptInstructions": {
		prepare: func(w *world) {
			w.Write(tenant, func(tx port.Tx) error {
				c, err := tx.GetConnector(w.conn.ID)
				if err != nil {
					return err
				}
				c.Config = map[string]string{"_instructions_pending": "sha256:next"}
				_, err = tx.PutConnector(c, c.Revision)
				return err
			})
		},
		act: func(w *world) { st.Ok(w.Svc.AcceptInstructions(w.Ctx, w.admin, w.conn.ID)) },
	},
	"ClassifyFunction": {act: func(w *world) {
		st.Ok(w.Svc.ClassifyFunction(w.Ctx, w.admin, w.conn.ID, "search", port.ClassWrite))
	}},
	"SetProjection": {prepare: projectable, act: func(w *world) {
		st.Ok(w.Svc.SetProjection(w.Ctx, w.admin, w.conn.ID, "search", true))
	}},
	"DisableFunction": {act: func(w *world) {
		st.Ok(w.Svc.SetFunctionDisabled(w.Ctx, w.admin, w.conn.ID, "search", true))
	}},
	"EnableFunction": {
		prepare: func(w *world) { st.Ok(w.Svc.SetFunctionDisabled(w.Ctx, w.admin, w.conn.ID, "search", true)) },
		act:     func(w *world) { st.Ok(w.Svc.SetFunctionDisabled(w.Ctx, w.admin, w.conn.ID, "search", false)) },
	},
}

func functionAdminOperations() []server.Operation {
	var out []server.Operation
	for _, o := range server.Operations() {
		if o.Method != "GET" && strings.HasPrefix(o.Path, "/v1/connectors/{id}/") && (strings.Contains(o.Path, "/functions") || strings.Contains(o.Path, "/instructions")) {
			out = append(out, o)
		}
	}
	return out
}

func TestEveryFunctionAdminActIsAudited(t *testing.T) {
	ops := functionAdminOperations()
	if len(ops) == 0 {
		t.Fatal("the operation table lists no function administration")
	}
	seen := map[string]bool{}
	for _, o := range ops {
		if seen[o.RPC] {
			continue
		}
		seen[o.RPC] = true
		t.Run(o.RPC, func(t *testing.T) {
			a, ok := functionActs[o.RPC]
			if !ok {
				t.Fatalf("%s %s administers functions and has no audit expectation", o.Method, o.Path)
			}
			w := newWorld(t, opts{})
			if a.prepare != nil {
				a.prepare(w)
			}
			var before []port.AuditRow
			w.Read(tenant, func(tx port.Tx) { before, _ = tx.ListAudit(string(w.conn.ID)) })
			events := len(w.Events())
			a.act(w)
			var after []port.AuditRow
			w.Read(tenant, func(tx port.Tx) { after, _ = tx.ListAudit(string(w.conn.ID)) })
			if len(after) <= len(before) {
				t.Fatalf("%s wrote no audit row", o.RPC)
			}
			row := after[len(after)-1]
			if row.Actor != string(w.admin.Person) || row.Subject != string(w.conn.ID) || row.Tenant != tenant || !catalog.Known(row.Action) {
				t.Fatalf("%s wrote %+v: want the admin, the connector and a published type", o.RPC, row)
			}
			if !slices.Contains(w.Types(events), row.Action) {
				t.Fatalf("%s audited %s without publishing it: %v", o.RPC, row.Action, w.Types(events))
			}
		})
	}
	for rpc := range functionActs {
		if !seen[rpc] {
			t.Fatalf("expectation for %s matches no operation in the table", rpc)
		}
	}
}
