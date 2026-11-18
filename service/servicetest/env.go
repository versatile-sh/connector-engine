package servicetest

import (
	"context"
	"fmt"
	"sync/atomic"
	"testing"
	"time"

	"github.com/agents-control/connector-engine/authz"
	"github.com/agents-control/connector-engine/authz/table"
	"github.com/agents-control/connector-engine/port"
	"github.com/agents-control/connector-engine/service"
	"github.com/agents-control/connector-engine/service/worker"
	"github.com/agents-control/connector-engine/store/memory"
)

// Env is a Service wired over the memory store, the table authorizer and the fakes, with a worker on the same clock.
type Env struct {
	T       *testing.T
	Ctx     context.Context
	Clock   *Clock
	Store   *memory.Store
	Secrets *Secrets
	Runtime *Runtime
	Auth    *table.Authorizer
	Svc     *service.Service
	Worker  *worker.Worker
	n       atomic.Int64
}

// Epoch is the fake time every Env starts at.
var Epoch = time.Date(2026, 11, 18, 9, 0, 0, 0, time.UTC)

// New builds an Env; tune may change the config and the dependencies before the Service is built.
func New(t *testing.T, tune ...func(*service.Config, *service.Deps)) *Env {
	t.Helper()
	e := &Env{T: t, Ctx: context.Background(), Clock: NewClock(Epoch), Store: memory.New(), Secrets: NewSecrets(), Runtime: NewRuntime()}
	cfg := service.DefaultConfig()
	e.Auth = table.New(e.Store, e.Clock, table.Settings{DestructiveMax: cfg.DestructiveMax})
	deps := service.Deps{Store: e.Store, Clock: e.Clock, Authorizer: e.Auth, Secrets: e.Secrets, Runtime: e.Runtime, NewID: e.id}
	for _, f := range tune {
		f(&cfg, &deps)
	}
	var err error
	if e.Svc, err = service.New(deps, cfg); err != nil {
		t.Fatal(err)
	}
	e.Worker = worker.New(e.Svc, e.Store, e.Clock, worker.Config{})
	return e
}

func (e *Env) id() string {
	return fmt.Sprintf("id%04d", e.n.Add(1))
}

// Operator is the system principal of the installation.
func (e *Env) Operator() port.Principal { return port.Principal{Kind: port.PrincipalSystem} }

// Person is a person principal of a tenant with the given role assignments (role, scope).
func (e *Env) Person(tenant port.TenantID, id string, grants ...[2]string) port.Principal {
	for _, g := range grants {
		e.Write(tenant, func(tx port.Tx) error {
			return tx.PutRole(port.RoleAssignment{Tenant: tenant, Person: port.PersonID(id), Role: g[0], Scope: g[1]})
		})
	}
	return port.Principal{Kind: port.PrincipalPerson, Person: port.PersonID(id), Claims: port.Claims{Tenant: tenant, Subject: id}}
}

// TenantAdmin is a person with the tenant admin role over the whole tenant.
func (e *Env) TenantAdmin(tenant port.TenantID) port.Principal {
	return e.Person(tenant, "admin", [2]string{string(table.TenantAdmin), authz.Tenant(string(tenant))})
}

// AgentClaims builds the principal of one run of an agent.
func (e *Env) AgentClaims(c port.Claims) port.Principal {
	return port.Principal{Kind: port.KindOf(c), Claims: c, Token: fmt.Sprintf("bearer-token-%d", e.n.Add(1))}
}

// Write runs fn on a write transaction of the tenant, or of the installation for the empty tenant.
func (e *Env) Write(tenant port.TenantID, fn func(port.Tx) error) {
	e.T.Helper()
	if err := e.Store.Write(e.Ctx, port.Scope{Tenant: tenant, Installation: tenant == ""}, fn); err != nil {
		e.T.Fatal(err)
	}
}

// Join makes a person a member of a group.
func (e *Env) Join(tenant port.TenantID, person port.PersonID, group port.GroupID) {
	e.T.Helper()
	e.Write(tenant, func(tx port.Tx) error {
		return tx.PutMember(port.GroupMember{Tenant: tenant, Person: person, Group: group})
	})
}

// Events returns every undelivered outbox event in order.
func (e *Env) Events() []port.Event {
	ev, err := e.Store.Pending(e.Ctx, 0)
	if err != nil {
		e.T.Fatal(err)
	}
	return ev
}

// Types returns the types of the undelivered events from index from on.
func (e *Env) Types(from int) []string {
	var out []string
	for _, ev := range e.Events()[from:] {
		out = append(out, ev.Type)
	}
	return out
}

// Read runs fn on a read transaction of the tenant.
func (e *Env) Read(tenant port.TenantID, fn func(port.Tx)) {
	e.T.Helper()
	if err := e.Store.Read(e.Ctx, port.Scope{Tenant: tenant, Installation: tenant == ""}, func(tx port.Tx) error { fn(tx); return nil }); err != nil {
		e.T.Fatal(err)
	}
}

// Ok returns v, or panics with err so the running test fails at the call site; it takes a call that returns (value, error).
func Ok[T any](v T, err error) T {
	if err != nil {
		panic(err)
	}
	return v
}
