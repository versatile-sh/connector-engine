package console_test

import (
	"context"
	"errors"
	"net/http"
	"net/http/cookiejar"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/versatile-sh/connector-engine/config"
	"github.com/versatile-sh/connector-engine/port"
	"github.com/versatile-sh/connector-engine/selfhosted/console"
	"github.com/versatile-sh/connector-engine/selfhosted/surface"
)

func (s *stack) acceptInvite(link linkOut, username string) *stack {
	s.t.Helper()
	jar, _ := cookiejar.New(nil)
	b := &browser{t: s.t, base: s.base, client: &http.Client{Jar: jar}}
	var out struct{ CSRF string }
	b.ok("POST", "/api/invite/accept", map[string]string{"token": link.token(s.t), "username": username, "password": "a brand new password"}, &out)
	b.csrf = out.CSRF
	other := *s
	other.b = b
	return &other
}

func mcpStatus(t *testing.T, tok console.AgentToken) int {
	t.Helper()
	req, _ := http.NewRequest("POST", tok.MCPURL, strings.NewReader(`{"jsonrpc":"2.0","id":1,"method":"ping"}`))
	req.Header.Set("Authorization", "Bearer "+tok.Token)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json, text/event-stream")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	return resp.StatusCode
}

func TestTwoPeopleJoinByInviteUseTheirOwnKeysAndTheOwnerRemovesOne(t *testing.T) {
	s := freshOwner(t, personRelays, func(c *config.Config) { c.Approval.HoldWindow = 10 * time.Second })
	var imported struct{ Connector string }
	s.b.ok("POST", "/api/import", map[string]any{"mcp": newKeyEchoServer(t).URL + "/mcp", "local": true, "header_secret": true, "choices": map[string]string{"whoami": "on", "send": "ask"}}, &imported)
	conn := imported.Connector
	s.b.ok("POST", "/api/connectors/"+conn+"/headers", map[string]any{"headers": map[string]string{"X-Api-Key": "org"}}, nil)
	var link linkOut
	s.b.ok("POST", "/api/invites", map[string]any{"role": "member"}, &link)
	owner := viewer{profiled{stack: s, conn: conn}}
	bob := viewer{profiled{stack: s.acceptInvite(link, "bob"), conn: conn}}
	bobP := ownerOf(t, bob.b)
	ownerKey, bobKey := owner.mint("ownerkey"), bob.mint("bobkey")
	if a, _ := s.c.State.Agent("bobkey"); a.Owner != bobP {
		t.Fatalf("the invited user's key belongs to %q, not %q", a.Owner, bobP)
	}

	t.Run("each sees only their own key and the Owner sees both under everyone", func(t *testing.T) {
		if got := names(bob.keys("?scope=everyone")); !slices.Equal(got, []string{"bobkey@bob"}) {
			t.Errorf("bob's keys %v", got)
		}
		if got := names(owner.keys("")); !slices.Equal(got, []string{"ownerkey@me"}) {
			t.Errorf("the owner's keys %v", got)
		}
		if got := names(owner.keys("?scope=everyone")); !slices.Equal(got, []string{"bobkey@bob", "ownerkey@me"}) {
			t.Errorf("everyone's keys %v", got)
		}
	})

	t.Run("each sees only their own calls in activity", func(t *testing.T) {
		callVault(t, ownerKey, "whoami")
		callVault(t, bobKey, "whoami")
		only := func(person string) func(console.ActivityPage) bool {
			return func(pg console.ActivityPage) bool {
				return len(pg.Calls) > 0 && !slices.ContainsFunc(pg.Calls, func(c console.ActivityCall) bool { return c.Owner.Person != person })
			}
		}
		bob.activityUntil("?scope=everyone", only(bobP))
		owner.activityUntil("", only("owner"))
		owner.activityUntil("?scope=everyone", func(pg console.ActivityPage) bool {
			return slices.ContainsFunc(pg.Calls, func(c console.ActivityCall) bool { return c.Owner.Person == bobP }) && slices.ContainsFunc(pg.Calls, func(c console.ActivityCall) bool { return c.Owner.Person == "owner" })
		})
	})

	t.Run("on a shared connector the Member and the Owner may both approve and the decider is recorded", func(t *testing.T) {
		h := hold(t, bob, bobKey)
		for who, v := range map[string]viewer{"bob": bob, "owner": owner} {
			if c := v.approval(h.id).Choices; !slices.Contains(c, surface.ChoiceOnce) {
				t.Errorf("%s is offered %v", who, c)
			}
		}
		if status, code := owner.decide(h.id, surface.ChoiceOnce); status != http.StatusOK {
			t.Fatalf("the Owner's approval answered %d %s", status, code)
		}
		h.wait(t)
		if a := decidedBy(t, s, h.id); a.DecidedBy != "owner" {
			t.Errorf("decided by %q", a.DecidedBy)
		}
		h = hold(t, bob, bobKey)
		if status, code := bob.decide(h.id, surface.ChoiceOnce); status != http.StatusOK {
			t.Fatalf("the Member's approval answered %d %s", status, code)
		}
		h.wait(t)
		if a := decidedBy(t, s, h.id); a.DecidedBy != bobP {
			t.Errorf("decided by %q", a.DecidedBy)
		}
	})

	t.Run("on a per-person connector only the Member approves and the Owner cancels", func(t *testing.T) {
		owner.b.ok("PUT", "/api/connectors/"+conn+"/credential-mode", map[string]any{"mode": console.CredentialPerPerson}, nil)
		bob.b.ok("POST", "/api/connectors/"+conn+"/headers", map[string]any{"headers": map[string]string{"X-Api-Key": "bob"}}, nil)
		h := hold(t, bob, bobKey)
		if status, code := owner.decide(h.id, surface.ChoiceOnce); status != http.StatusForbidden || code != "not_an_approver" {
			t.Errorf("the Owner's approval answered %d %s", status, code)
		}
		owner.b.ok("POST", "/api/approvals/"+h.id+"/cancel", nil, nil)
		h.wait(t)
		if a := decidedBy(t, s, h.id); a.State != port.ApprovalCancelled {
			t.Errorf("the call is %s", a.State)
		}
		owner.b.ok("PUT", "/api/connectors/"+conn+"/credential-mode", map[string]any{"mode": console.CredentialShared}, nil)
	})

	t.Run("the Member is refused what only managers do and finds the Owner's rows missing", func(t *testing.T) {
		for _, c := range []struct {
			method, path string
			body         any
		}{
			{"POST", "/api/import", map[string]any{"mcp": "https://example.com/mcp"}},
			{"PUT", "/api/connectors/" + conn + "/functions/whoami", map[string]string{"access": "off"}},
			{"POST", "/api/invites", map[string]string{"role": "member"}},
			{"POST", "/api/people/owner/reset", nil},
			{"PUT", "/api/people/owner/role", map[string]string{"role": "member"}},
			{"POST", "/api/backup/export", map[string]any{"mode": "full", "password": ownerPassword, "passphrase": passphrase, "confirm": true}},
			{"DELETE", "/api/slots/any", nil},
		} {
			bob.refusedAs(t, http.StatusForbidden, c.method, c.path, c.body)
		}
		h := hold(t, owner, ownerKey)
		bob.refusedAs(t, http.StatusNotFound, "GET", "/api/approvals/"+h.id, nil)
		bob.refusedAs(t, http.StatusNotFound, "DELETE", "/api/agents/ownerkey", nil)
		owner.b.ok("POST", "/api/approvals/"+h.id+"/cancel", nil, nil)
		h.wait(t)
	})

	t.Run("the last Owner can be neither demoted nor removed", func(t *testing.T) {
		owner.refusedAs(t, http.StatusConflict, "PUT", "/api/people/owner/role", map[string]string{"role": "member"})
		owner.refusedAs(t, http.StatusConflict, "DELETE", "/api/people/owner", nil)
	})

	t.Run("a role change reaches the engine and the next request", func(t *testing.T) {
		if status, _ := bob.b.send("GET", "/api/people", nil, nil); status != http.StatusForbidden {
			t.Fatalf("a Member reading people: %d", status)
		}
		owner.b.ok("PUT", "/api/people/"+bobP+"/role", map[string]string{"role": "admin"}, nil)
		if status, _ := bob.b.send("GET", "/api/people", nil, nil); status != http.StatusOK {
			t.Errorf("the promoted user reading people: %d", status)
		}
		owner.b.ok("PUT", "/api/people/"+bobP+"/role", map[string]string{"role": "member"}, nil)
		if status, _ := bob.b.send("GET", "/api/people", nil, nil); status != http.StatusForbidden {
			t.Errorf("the demoted user reading people: %d", status)
		}
		found := false
		for _, a := range s.c.State.Audit(50) {
			found = found || a.Action == "role.changed" && a.Target == bobP && a.Actor == "owner" && a.Detail["from"] == "admin" && a.Detail["to"] == "member"
		}
		if !found {
			t.Error("the audit lacks the demotion")
		}
	})

	t.Run("removing the Member signs them out and stops their key everywhere", func(t *testing.T) {
		if mcpStatus(t, bobKey) == http.StatusUnauthorized {
			t.Fatal("bob's key is refused before the removal")
		}
		owner.b.ok("DELETE", "/api/people/"+bobP, nil, nil)
		bob.refusedAs(t, http.StatusUnauthorized, "GET", "/api/agents", nil)
		if status := mcpStatus(t, bobKey); status != http.StatusUnauthorized {
			t.Errorf("bob's key at /mcp answered %d", status)
		}
		err := s.c.Engine.Call(console.WithActor(context.Background(), bobP), http.MethodPut, "/v1/roles", port.RoleAssignment{Tenant: "local", Person: port.PersonID(bobP), Role: "tenant_admin", Scope: "t/local"}, nil)
		var refused *console.EngineError
		if !errors.As(err, &refused) || refused.Status != http.StatusForbidden && refused.Status != http.StatusUnauthorized {
			t.Errorf("the engine lets the removed person write: %v", err)
		}
		if got := names(owner.keys("?scope=everyone")); !slices.Equal(got, []string{"ownerkey@me"}) {
			t.Errorf("the keys after the removal %v", got)
		}
		if _, ok := s.c.State.User(bobP); ok {
			t.Error("the user is still in the state")
		}
		actions := []string{}
		for _, a := range s.c.State.Audit(50) {
			actions = append(actions, a.Action)
		}
		if actions[0] != "user.removed" || !slices.Contains(actions, "user.created") || !slices.Contains(actions, "invite.created") {
			t.Errorf("audit %v", actions)
		}
		if mcpStatus(t, ownerKey) == http.StatusUnauthorized {
			t.Error("the removal stopped the Owner's key")
		}
	})
}
