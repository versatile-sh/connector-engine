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

type adminAct struct {
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

var functionActs = map[string]adminAct{
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

func operationsUnder(match func(server.Operation) bool) []server.Operation {
	var out []server.Operation
	for _, o := range server.Operations() {
		if o.Method != "GET" && match(o) {
			out = append(out, o)
		}
	}
	return out
}

func requireAudited(t *testing.T, ops []server.Operation, acts map[string]adminAct, subject func(*world) string, published bool) {
	t.Helper()
	if len(ops) == 0 {
		t.Fatal("the operation table lists no such administration")
	}
	seen := map[string]bool{}
	for _, o := range ops {
		if seen[o.RPC] {
			continue
		}
		seen[o.RPC] = true
		t.Run(o.RPC, func(t *testing.T) {
			a, ok := acts[o.RPC]
			if !ok {
				t.Fatalf("%s %s is administration and has no audit expectation", o.Method, o.Path)
			}
			w := newWorld(t, opts{})
			if a.prepare != nil {
				a.prepare(w)
			}
			rows := func() (out []port.AuditRow) {
				w.Read(tenant, func(tx port.Tx) { out, _ = tx.ListAudit(subject(w)) })
				return out
			}
			before, events := rows(), len(w.Events())
			a.act(w)
			after := rows()
			if len(after) <= len(before) {
				t.Fatalf("%s wrote no audit row", o.RPC)
			}
			row := after[len(after)-1]
			if row.Actor != string(w.admin.Person) || row.Subject != subject(w) || row.Tenant != tenant {
				t.Fatalf("%s wrote %+v: want the admin as actor, the subject and the tenant", o.RPC, row)
			}
			if published && (!catalog.Known(row.Action) || !slices.Contains(w.Types(events), row.Action)) {
				t.Fatalf("%s audited %s without publishing a catalog type: %v", o.RPC, row.Action, w.Types(events))
			}
		})
	}
	for rpc := range acts {
		if !seen[rpc] {
			t.Fatalf("expectation for %s matches no operation in the table", rpc)
		}
	}
}

func TestEveryFunctionAdminActIsAudited(t *testing.T) {
	ops := operationsUnder(func(o server.Operation) bool {
		return strings.HasPrefix(o.Path, "/v1/connectors/{id}/") && (strings.Contains(o.Path, "/functions") || strings.Contains(o.Path, "/instructions"))
	})
	requireAudited(t, ops, functionActs, func(w *world) string { return string(w.conn.ID) }, true)
}

var roleActs = map[string]adminAct{
	"PutRole": {act: func(w *world) {
		st.Ok(w.Svc.PutRole(w.Ctx, w.admin, port.RoleAssignment{Tenant: tenant, Person: "bia", Role: "approver", Scope: "t/t1/w/w1"}))
	}},
	"DeleteRole": {
		prepare: func(w *world) {
			st.Ok(w.Svc.PutRole(w.Ctx, w.admin, port.RoleAssignment{Tenant: tenant, Person: "bia", Role: "approver", Scope: "t/t1/w/w1"}))
		},
		act: func(w *world) {
			if err := w.Svc.DeleteRole(w.Ctx, w.admin, port.RoleAssignment{Tenant: tenant, Person: "bia", Role: "approver", Scope: "t/t1/w/w1"}); err != nil {
				w.t.Fatal(err)
			}
		},
	},
	"PutGroupMember": {act: func(w *world) {
		st.Ok(w.Svc.PutGroupMember(w.Ctx, w.admin, port.GroupMember{Person: "bia", Group: "ops"}))
	}},
	"DeleteGroupMember": {
		prepare: func(w *world) {
			st.Ok(w.Svc.PutGroupMember(w.Ctx, w.admin, port.GroupMember{Person: "bia", Group: "ops"}))
		},
		act: func(w *world) {
			if err := w.Svc.DeleteGroupMember(w.Ctx, w.admin, port.GroupMember{Person: "bia", Group: "ops"}); err != nil {
				w.t.Fatal(err)
			}
		},
	},
}

func TestEveryRoleAndGroupActIsAudited(t *testing.T) {
	ops := operationsUnder(func(o server.Operation) bool {
		return strings.HasPrefix(o.Path, "/v1/roles") || strings.HasPrefix(o.Path, "/v1/groups")
	})
	requireAudited(t, ops, roleActs, func(*world) string { return "bia" }, false)
}
