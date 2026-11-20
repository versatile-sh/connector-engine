package console_test

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"

	"github.com/versatile-sh/connector-engine/port"
	"github.com/versatile-sh/connector-engine/selfhosted/console"
)

func (s *stack) signInAs(person, username string, role console.Role) *stack {
	s.t.Helper()
	console.AddTestUser(s.t, s.c.State, person, username, ownerPassword, role)
	if err := s.c.ReconcileRoles(context.Background()); err != nil {
		s.t.Fatal(err)
	}
	jar, _ := cookiejar.New(nil)
	b := &browser{t: s.t, base: s.base, client: &http.Client{Jar: jar}}
	var out struct{ CSRF string }
	b.ok("POST", "/api/login", map[string]string{"username": username, "password": ownerPassword}, &out)
	b.csrf = out.CSRF
	other := *s
	other.b = b
	return &other
}

func claimsOf(t *testing.T, token string) map[string]any {
	t.Helper()
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		t.Fatalf("not a JWT: %q", token)
	}
	raw, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		t.Fatal(err)
	}
	var claims map[string]any
	if err := json.Unmarshal(raw, &claims); err != nil {
		t.Fatal(err)
	}
	return claims
}

func TestAKeyMadeByASecondUserActsOnBehalfOfThatUser(t *testing.T) {
	p := profiledStack(t)
	bob := profiled{stack: p.signInAs("p-bob", "bob", console.RoleMember), conn: p.conn}
	tok := bob.mint("bobkey")
	if c := claimsOf(t, tok.Token); c["on_behalf_of"] != "p-bob" || c["agent_owner"] != "p-bob" {
		t.Fatalf("the key's token names %v and %v", c["on_behalf_of"], c["agent_owner"])
	}
	if a, _ := p.c.State.Agent("bobkey"); a.Owner != "p-bob" {
		t.Fatalf("the key's owner is %q", a.Owner)
	}
	var binds struct{ Items []port.Binding }
	if err := p.c.Engine.Read(context.Background(), http.MethodGet, "/v1/connectors/"+p.conn+"/bindings", nil, &binds); err != nil {
		t.Fatal(err)
	}
	if i := slices.IndexFunc(binds.Items, func(b port.Binding) bool { return b.Agent == "bobkey" }); i < 0 || binds.Items[i].AgentOwner != "p-bob" {
		t.Fatalf("the key's binding %+v", binds.Items)
	}
	if !callsOK(tok, "lookup") {
		t.Fatal("the second user's key cannot call")
	}

	t.Run("another person's key name is taken", func(t *testing.T) {
		p.mint("alpha")
		if status, _ := bob.b.send("POST", "/api/agents", map[string]any{"name": "alpha"}, nil); status != http.StatusConflict {
			t.Fatalf("renewing another person's key answered %d", status)
		}
		if a, _ := p.c.State.Agent("alpha"); a.Owner != "owner" {
			t.Fatalf("alpha changed owner to %q", a.Owner)
		}
	})

	t.Run("the deciding user is recorded on the approval", func(t *testing.T) {
		p.narrow("bobkey", "lookup", console.AccessAsk)
		ran := make(chan bool, 1)
		go func() { ran <- callsOK(tok, "lookup") }()
		held := bob.heldFor("bobkey")
		bob.b.ok("POST", "/api/approvals/"+held, map[string]any{"approve": true}, nil)
		if !<-ran {
			t.Fatal("the approved call did not run")
		}
		var d struct{ Approval port.Approval }
		if err := p.c.Engine.Read(context.Background(), http.MethodGet, "/v1/approvals/"+held, nil, &d); err != nil {
			t.Fatal(err)
		}
		if d.Approval.DecidedBy != "p-bob" {
			t.Fatalf("decided by %q", d.Approval.DecidedBy)
		}
	})
}

func TestAPermissionSetClaimedByASecondUserLandsOnTheirKey(t *testing.T) {
	dst := importedSlots(t, slotSetup(t))
	bob := profiled{stack: dst.signInAs("p-bob", "bob", console.RoleMember), conn: dst.conn}
	agent := bob.consent("alpha")
	if a, _ := dst.c.State.Agent(agent); a.Owner != "p-bob" {
		t.Fatalf("the app's key belongs to %q", a.Owner)
	}
	dst.hasExactly(agent, console.ProfileCustom, map[string]console.Access{"lookup": console.AccessAsk})
	bob.b.ok("POST", "/api/agents", map[string]any{"name": "cc", "slot": "beta"}, nil)
	if a, _ := dst.c.State.Agent("cc"); a.Owner != "p-bob" {
		t.Fatalf("the hand-made key belongs to %q", a.Owner)
	}
	dst.hasExactly("cc", console.ProfileOff, nil)
}

func TestReconcileRepairsTheEngineRolesAndTheManagersGroup(t *testing.T) {
	s := freshOwner(t)
	console.AddTestUser(t, s.c.State, "p-ann", "ann", ownerPassword, console.RoleAdmin)
	console.AddTestUser(t, s.c.State, "p-bob", "bob", ownerPassword, console.RoleMember)
	ctx := context.Background()
	if err := s.c.ReconcileRoles(ctx); err != nil {
		t.Fatal(err)
	}
	service := console.WithActor(ctx, "console")
	for _, path := range []string{"/v1/roles?tenant=local&person=owner&role=tenant_admin&scope=t/local", "/v1/groups/members?tenant=local&person=p-ann&group=managers"} {
		if err := s.c.Engine.Call(service, http.MethodDelete, path, nil, nil); err != nil {
			t.Fatal(path, err)
		}
	}
	for _, r := range []port.RoleAssignment{{Tenant: "local", Person: "p-bob", Role: "tenant_admin", Scope: "t/local"}, {Tenant: "local", Person: "ops", Role: "auditor", Scope: "t/local"}} {
		if err := s.c.Engine.Call(service, http.MethodPut, "/v1/roles", r, nil); err != nil {
			t.Fatal(err)
		}
	}
	for _, person := range []port.PersonID{"p-bob", "ops"} {
		if err := s.c.Engine.Call(service, http.MethodPut, "/v1/groups/members", port.GroupMember{Person: person, Group: "managers"}, nil); err != nil {
			t.Fatal(err)
		}
	}
	if err := s.c.ReconcileRoles(ctx); err != nil {
		t.Fatal(err)
	}
	if got, want := engineRoles(t, s, ""), []string{"console tenant_admin t/local", "ops auditor t/local", "owner tenant_admin t/local", "p-ann tenant_admin t/local", "p-bob agent_configurer t/local/w/default"}; !slices.Equal(got, want) {
		t.Errorf("roles %v, want %v", got, want)
	}
	var members struct{ Items []port.GroupMember }
	if err := s.c.Engine.Read(ctx, http.MethodGet, "/v1/groups/members?group=managers", nil, &members); err != nil {
		t.Fatal(err)
	}
	names := []string{}
	for _, m := range members.Items {
		names = append(names, string(m.Person))
	}
	slices.Sort(names)
	if want := []string{"ops", "owner", "p-ann"}; !slices.Equal(names, want) {
		t.Errorf("managers %v, want %v", names, want)
	}

	t.Run("a role change keeps the last owner and moves the engine row", func(t *testing.T) {
		if err := s.c.SetUserRole(ctx, "owner", console.RoleAdmin); !errors.Is(err, console.ErrLastOwner) {
			t.Fatalf("demoting the last owner: %v", err)
		}
		if got := engineRoles(t, s, "owner"); !slices.Equal(got, []string{"owner tenant_admin t/local"}) {
			t.Fatalf("a refused demotion changed the engine: %v", got)
		}
		if err := s.c.SetUserRole(ctx, "p-ann", console.RoleMember); err != nil {
			t.Fatal(err)
		}
		if u, _ := s.c.State.User("p-ann"); u.Role != console.RoleMember {
			t.Fatalf("ann is %s", u.Role)
		}
		if got := engineRoles(t, s, "p-ann"); !slices.Equal(got, []string{"p-ann agent_configurer t/local/w/default"}) {
			t.Fatalf("ann's engine rows %v", got)
		}
		if err := s.c.SetUserRole(ctx, "p-bob", console.RoleAdmin); err != nil {
			t.Fatal(err)
		}
		if got := engineRoles(t, s, "p-bob"); !slices.Equal(got, []string{"p-bob tenant_admin t/local"}) {
			t.Fatalf("bob's engine rows %v", got)
		}
	})
}

func engineRoles(t *testing.T, s *stack, person string) []string {
	t.Helper()
	var roles struct{ Items []port.RoleAssignment }
	path := "/v1/roles?tenant=local"
	if person != "" {
		path += "&person=" + person
	}
	if err := s.c.Engine.Read(context.Background(), http.MethodGet, path, nil, &roles); err != nil {
		t.Fatal(err)
	}
	got := []string{}
	for _, r := range roles.Items {
		got = append(got, string(r.Person)+" "+r.Role+" "+r.Scope)
	}
	slices.Sort(got)
	return got
}

func TestASetupImportedFromTheHostActsAsAnOwner(t *testing.T) {
	file := profiledStack(t).exportSetup()
	dst := freshOwner(t, personRelays)
	rec := httptest.NewRecorder()
	dst.c.HostHandler().ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/import", strings.NewReader("\n"+string(file))))
	if rec.Code != http.StatusOK {
		t.Fatalf("the host import answered %d %s", rec.Code, rec.Body)
	}
	var conns struct{ Connectors []console.ConnectorSummary }
	dst.b.ok("GET", "/api/connectors", nil, &conns)
	if len(conns.Connectors) != 1 {
		t.Fatalf("connectors after the host import %+v", conns.Connectors)
	}
}
