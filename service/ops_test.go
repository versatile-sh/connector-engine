package service_test

import (
	"testing"

	"github.com/agents-control/connector-engine/credential"
	"github.com/agents-control/connector-engine/port"
	"github.com/agents-control/connector-engine/service"
	st "github.com/agents-control/connector-engine/service/servicetest"
	srcmemory "github.com/agents-control/connector-engine/source/memory"
)

func TestListCallsIsAuditOnlyAndPerTenant(t *testing.T) {
	w := newWorld(t, opts{})
	w.Write(tenant, func(tx port.Tx) error {
		for _, c := range []port.Call{
			{ID: "k1", Tenant: tenant, Agent: "x", Connector: w.conn.ID, State: port.CallFinished, StartedAt: st.Epoch},
			{ID: "k2", Tenant: tenant, Agent: "y", Connector: w.conn.ID, State: port.CallStarted, StartedAt: st.Epoch.Add(1)},
		} {
			if err := tx.CreateCall(c); err != nil {
				return err
			}
		}
		return nil
	})
	got := st.Ok(w.Svc.ListCalls(w.Ctx, w.admin, port.CallFilter{}))
	want(t, len(got), 2, "an admin lists every call")
	got = st.Ok(w.Svc.ListCalls(w.Ctx, w.admin, port.CallFilter{Agent: "y"}))
	want(t, len(got) == 1 && got[0].ID == "k2", true, "filtered by agent")
	_, err := w.Svc.ListCalls(w.Ctx, w.alice, port.CallFilter{})
	want(t, port.IsCode(err, port.CodePermissionDenied), true, "a person without audit role may not list calls")
	other := w.TenantAdmin("t2")
	st.Ok(w.Svc.CreateTenant(w.Ctx, w.Operator(), "t2"))
	want(t, len(st.Ok(w.Svc.ListCalls(w.Ctx, other, port.CallFilter{}))), 0, "another tenant sees none")
}

func TestListSourcesShowsTheLastSync(t *testing.T) {
	m := st.Manifest("acme.dev/imported", "1.0.0")
	tune := func(_ *service.Config, d *service.Deps) {
		d.Sources = map[string]port.ListingSource{"fake": srcmemory.New("fake", 1, m)}
	}
	w := newWorld(t, opts{tune: []func(*service.Config, *service.Deps){tune}})
	rows := st.Ok(w.Svc.ListSources(w.Ctx, w.admin))
	want(t, len(rows) == 1 && rows[0].Name == "fake" && rows[0].LastSync.LastSync.IsZero(), true, "never synced")
	st.Ok(w.Svc.SyncSource(w.Ctx, w.Operator(), "fake"))
	rows = st.Ok(w.Svc.ListSources(w.Ctx, w.admin))
	want(t, rows[0].LastSync.Imported, 1, "one imported")
	want(t, rows[0].LastSync.LastSync.Equal(st.Epoch), true, "at the sync time")
	_, err := w.Svc.ListSources(w.Ctx, w.alice)
	want(t, port.IsCode(err, port.CodePermissionDenied), true, "admin only")
}

func TestRotateKeyIsTheOperatorsAndValidatesItsScope(t *testing.T) {
	w := newWorld(t, opts{})
	op := st.Ok(w.Svc.RotateKey(w.Ctx, w.Operator(), port.KeyScope{Root: true}))
	want(t, op.Done && op.Kind == "rotate_key", true, "root rotation done")
	st.Ok(w.Svc.RotateKey(w.Ctx, w.Operator(), port.KeyScope{Tenant: tenant}))
	_, err := w.Svc.RotateKey(w.Ctx, w.Operator(), port.KeyScope{Tenant: "ghost"})
	want(t, port.IsCode(err, port.CodeNotFound), true, "an unknown tenant")
	_, err = w.Svc.RotateKey(w.Ctx, w.Operator(), port.KeyScope{})
	want(t, port.IsCode(err, port.CodeInvalidArgument), true, "no scope")
	_, err = w.Svc.RotateKey(w.Ctx, w.Operator(), port.KeyScope{Root: true, Tenant: tenant})
	want(t, port.IsCode(err, port.CodeInvalidArgument), true, "both scopes")
	_, err = w.Svc.RotateKey(w.Ctx, w.admin, port.KeyScope{Root: true})
	want(t, port.IsCode(err, port.CodePermissionDenied), true, "a tenant admin may not rotate")
}

func TestClientRegistrationIsSealedAndFeedsTheRegistrar(t *testing.T) {
	w := newWorld(t, opts{})
	got := st.Ok(w.Svc.PutClientRegistration(w.Ctx, w.admin, service.ClientRegistrationInput{Issuer: "https://idp.example", ClientID: "cid", Secret: "s3cret", RedirectURL: "https://engine/cb"}))
	want(t, got.Secret == nil && got.ClientID == "cid", true, "the answer carries no secret")
	for _, bad := range []string{"http://insecure", "https://", "https://user@idp.example", "https://idp.example?x=1"} {
		if _, err := w.Svc.PutClientRegistration(w.Ctx, w.admin, service.ClientRegistrationInput{Issuer: bad, ClientID: "c"}); !port.IsCode(err, port.CodeInvalidArgument) {
			t.Fatalf("issuer %q: %v", bad, err)
		}
	}
	_, err := w.Svc.PutClientRegistration(w.Ctx, w.admin, service.ClientRegistrationInput{Issuer: "http://insecure", ClientID: "c"})
	want(t, port.IsCode(err, port.CodeInvalidArgument), true, "an https issuer is required")
	_, err = w.Svc.PutClientRegistration(w.Ctx, w.alice, service.ClientRegistrationInput{Issuer: "https://idp.example", ClientID: "c"})
	want(t, port.IsCode(err, port.CodePermissionDenied), true, "admin only")

	reg := credential.NewRegistrar().WithStore(w.Store, w.Secrets)
	c := st.Ok(reg.Resolve(w.Ctx, tenant, "https://idp.example"))
	want(t, c.ClientID+"/"+string(c.ClientSecret), "cid/s3cret", "the registrar opens the sealed client")
	want(t, c.Registration, port.RegistrationPreRegistered, "marked pre-registered")
	_, err = reg.Resolve(w.Ctx, "t2", "https://idp.example")
	want(t, port.IsCode(err, port.CodeNotFound), true, "another tenant has none")
}
