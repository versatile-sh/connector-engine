package service_test

import (
	"context"
	"fmt"
	"testing"

	"github.com/versatile-sh/connector-engine/authz"
	"github.com/versatile-sh/connector-engine/authz/table"
	"github.com/versatile-sh/connector-engine/port"
	"github.com/versatile-sh/connector-engine/service"
	st "github.com/versatile-sh/connector-engine/service/servicetest"
	"github.com/versatile-sh/connector-engine/store/memory"
	"github.com/versatile-sh/connector-engine/store/postgres"
	"github.com/versatile-sh/connector-engine/store/storetest"
)

const benchAgents, benchConnectors = 20, 20

type bench struct {
	svc   *service.Service
	admin port.Principal
	conns []port.ConnectorID
}

func seed(b *testing.B, store port.Store) bench {
	b.Helper()
	ctx, clock := context.Background(), st.NewClock(st.Epoch)
	rt := st.NewRuntime()
	cfg := service.DefaultConfig()
	auth := table.New(store, clock, table.Settings{DestructiveMax: cfg.DestructiveMax})
	svc, err := service.New(service.Deps{Store: store, Clock: clock, Authorizer: auth, Secrets: st.NewSecrets(), Runtime: rt}, cfg)
	if err != nil {
		b.Fatal(err)
	}
	must := func(err error) {
		b.Helper()
		if err != nil {
			b.Fatal(err)
		}
	}
	op := port.Principal{Kind: port.PrincipalSystem}
	_, err = svc.CreateTenant(ctx, op, tenant)
	must(err)
	must(store.Write(ctx, port.Scope{Tenant: tenant}, func(tx port.Tx) error {
		return tx.PutRole(port.RoleAssignment{Tenant: tenant, Person: "admin", Role: string(table.TenantAdmin), Scope: authz.Tenant(string(tenant))})
	}))
	admin := port.Principal{Kind: port.PrincipalPerson, Person: "admin", Claims: port.Claims{Tenant: tenant, Subject: "admin"}}
	defs, hashes, list := definitions("Searches", "Creates an issue")
	_ = defs
	rt.SetDefinitions(listing, list...)
	_, err = svc.CreateListing(ctx, op, service.CreateListingInput{Manifest: st.Manifest(listing, "1.0.0")})
	must(err)
	_, err = svc.ReviewListing(ctx, op, port.VersionRef{Listing: ref, Version: "1.0.0"}, service.ReviewInput{Functions: []service.ReviewFunction{
		{Name: "search", Hash: hashes["search"], Class: port.ClassRead}, {Name: "create_issue", Hash: hashes["create_issue"], Class: port.ClassWrite},
		{Name: "delete_repo", Hash: hashes["delete_repo"], Class: port.ClassDestructive}}})
	must(err)
	for _, r := range []port.CatalogRule{
		{SubjectKind: port.SubjectTenant, SubjectID: string(tenant), TargetKind: port.TargetListing, Target: ref.String(), Show: true, Install: true, Ceiling: port.CeilingFull},
		{ID: "r-ws", SubjectKind: port.SubjectWorkspace, SubjectID: string(workspace), TargetKind: port.TargetListing, Target: ref.String(), Show: true, Install: true, Ceiling: port.CeilingFull},
	} {
		_, err = svc.PutCatalogRule(ctx, admin, r)
		must(err)
	}
	out := bench{svc: svc, admin: admin}
	for i := range benchConnectors {
		v, err := svc.InstallConnector(ctx, admin, service.InstallInput{Workspace: workspace, Alias: fmt.Sprintf("c%02d", i), Listing: ref})
		must(err)
		out.conns = append(out.conns, v.Connector.ID)
	}
	must(store.Write(ctx, port.Scope{Tenant: tenant}, func(tx port.Tx) error {
		for a := range benchAgents {
			agent := port.AgentID(fmt.Sprintf("a%02d", a))
			for i, c := range out.conns {
				if err := tx.PutBinding(port.Binding{Tenant: tenant, Connector: c, Agent: agent}); err != nil {
					return err
				}
				g := port.Grant{ID: port.GrantID(fmt.Sprintf("g-%s-%02d", agent, i)), Tenant: tenant, ScopeKind: port.ScopeAgent, ScopeID: string(agent), Agent: agent,
					Connector: c, Selector: "*", Mode: port.ModeAllow, CreatedAt: st.Epoch, Origin: port.OriginSettings}
				if err := tx.PutGrant(g); err != nil {
					return err
				}
			}
		}
		return nil
	}))
	return out
}

func benchmarkStores(b *testing.B, run func(*testing.B, bench)) {
	b.Run("memory", func(b *testing.B) { run(b, seed(b, memory.New())) })
	b.Run("postgres", func(b *testing.B) {
		pg, err := storetest.StartPostgres()
		if err != nil {
			b.Skip(err)
		}
		b.Cleanup(pg.Stop)
		dsn, err := pg.NewDatabase()
		if err != nil {
			b.Fatal(err)
		}
		s, err := postgres.New(context.Background(), dsn)
		if err != nil {
			b.Fatal(err)
		}
		b.Cleanup(s.Close)
		if err := s.Migrate(context.Background()); err != nil {
			b.Fatal(err)
		}
		run(b, seed(b, s))
	})
}

func BenchmarkPutBindingWithManyAgentsAndConnectors(b *testing.B) {
	benchmarkStores(b, func(b *testing.B, w bench) {
		ctx := context.Background()
		for i := 0; b.Loop(); i++ {
			if _, err := w.svc.PutBinding(ctx, w.admin, port.Binding{Connector: w.conns[i%benchConnectors], Agent: "a00", AskRequester: true}); err != nil {
				b.Fatal(err)
			}
		}
	})
}

func BenchmarkCreateGrantWithManyAgentsAndConnectors(b *testing.B) {
	benchmarkStores(b, func(b *testing.B, w bench) {
		ctx := context.Background()
		for i := 0; b.Loop(); i++ {
			g := port.Grant{ScopeKind: port.ScopeAgent, ScopeID: "a00", Connector: w.conns[i%benchConnectors], Selector: "class:read", Mode: port.ModeAsk}
			if _, err := w.svc.CreateGrant(ctx, w.admin, g); err != nil {
				b.Fatal(err)
			}
		}
	})
}
