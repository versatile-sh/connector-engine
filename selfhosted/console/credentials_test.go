package console_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/versatile-sh/connector-engine/config"
	"github.com/versatile-sh/connector-engine/port"
	"github.com/versatile-sh/connector-engine/selfhosted/console"
	"github.com/versatile-sh/connector-engine/selfhosted/surface"
)

func newKeyEchoServer(t *testing.T) *httptest.Server {
	t.Helper()
	srv := mcp.NewServer(&mcp.Implementation{Name: "vault", Version: "1.0.0"}, nil)
	for _, name := range []string{"whoami", "send"} {
		srv.AddTool(&mcp.Tool{Name: name, Description: "answers with the key it got", Annotations: &mcp.ToolAnnotations{ReadOnlyHint: true}, InputSchema: map[string]any{"type": "object"}},
			func(_ context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
				return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: "key=" + req.Extra.Header.Get("X-Api-Key")}}}, nil
			})
	}
	u := httptest.NewServer(mcp.NewStreamableHTTPHandler(func(*http.Request) *mcp.Server { return srv }, nil))
	t.Cleanup(u.Close)
	return u
}

func callVault(t *testing.T, tok console.AgentToken, tool string) string {
	t.Helper()
	cs, err := mcp.NewClient(&mcp.Implementation{Name: "agent", Version: "1"}, nil).Connect(context.Background(), &mcp.StreamableClientTransport{Endpoint: tok.MCPURL, HTTPClient: &http.Client{Transport: bearer(tok.Token)}}, nil)
	if err != nil {
		t.Errorf("connect as %s: %v", tok.Agent, err)
		return ""
	}
	defer cs.Close()
	res, err := cs.CallTool(context.Background(), &mcp.CallToolParams{Name: "vault__" + tool, Arguments: map[string]any{}})
	if err != nil {
		return "error: " + err.Error()
	}
	return textOf(res)
}

type viewer struct{ profiled }

func (v viewer) approval(id string) console.ApprovalView {
	v.t.Helper()
	var out console.ApprovalView
	v.b.ok("GET", "/api/approvals/"+id, nil, &out)
	return out
}

func (v viewer) decide(id string, choice surface.Choice) (int, string) {
	v.t.Helper()
	status, raw := v.b.send("POST", "/api/approvals/"+id+"/decide", map[string]any{"choice": choice}, nil)
	var e struct{ Code string }
	_ = json.Unmarshal(raw, &e)
	return status, e.Code
}

func decidedBy(t *testing.T, s *stack, id string) port.Approval {
	t.Helper()
	var d struct{ Approval port.Approval }
	if err := s.c.Engine.Read(context.Background(), http.MethodGet, "/v1/approvals/"+id, nil, &d); err != nil {
		t.Fatal(err)
	}
	return d.Approval
}

type held struct {
	id     string
	answer chan string
}

func hold(t *testing.T, v viewer, tok console.AgentToken) held {
	t.Helper()
	h := held{answer: make(chan string, 1)}
	go func() { h.answer <- callVault(t, tok, "send") }()
	h.id = v.heldFor(tok.Agent)
	return h
}

func (h held) wait(t *testing.T) string {
	t.Helper()
	select {
	case a := <-h.answer:
		return a
	case <-time.After(20 * time.Second):
		t.Fatalf("the held call %s never answered", h.id)
		return ""
	}
}

func TestEachConnectorDecidesWhoseCredentialItsCallsUseAndWhoApprovesThem(t *testing.T) {
	s := freshOwner(t, personRelays, func(c *config.Config) { c.Approval.HoldWindow = 10 * time.Second })
	var imported struct{ Connector string }
	s.b.ok("POST", "/api/import", map[string]any{"mcp": newKeyEchoServer(t).URL + "/mcp", "local": true, "header_secret": true, "choices": map[string]string{"whoami": "on", "send": "ask"}}, &imported)
	s.b.trustReads(imported.Connector, "whoami", "send")
	conn := imported.Connector
	s.b.ok("POST", "/api/connectors/"+conn+"/headers", map[string]any{"headers": map[string]string{"X-Api-Key": "org"}}, nil)
	owner := viewer{profiled{stack: s, conn: conn}}
	ann := viewer{profiled{stack: s.signInAs("p-ann", "ann", console.RoleAdmin), conn: conn}}
	bob := viewer{profiled{stack: s.signInAs("p-bob", "bob", console.RoleMember), conn: conn}}
	ownerKey, bobKey := owner.mint("ownerkey"), bob.mint("bobkey")
	mode := func(m console.CredentialMode) {
		t.Helper()
		owner.b.ok("PUT", "/api/connectors/"+conn+"/credential-mode", map[string]any{"mode": m}, nil)
	}

	sum := owner.summary()
	if sum.Mode != console.CredentialShared || !slices.Equal(sum.Modes, []console.CredentialMode{console.CredentialShared, console.CredentialPerPerson}) || sum.People == nil || *sum.People != 3 {
		t.Fatalf("a new connector's summary %+v", sum)
	}
	if b := bob.summary(); b.People != nil || b.SignedInN != nil {
		t.Fatalf("a Member sees the sign-in counts %+v", b)
	}

	var before held
	t.Run("on a shared connector an Admin approves a Member's call", func(t *testing.T) {
		if got := callVault(t, bobKey, "whoami"); got != "key=org" {
			t.Fatalf("the Member's call used %q", got)
		}
		h := hold(t, bob, bobKey)
		if v := ann.approval(h.id); !slices.Contains(v.Choices, surface.ChoiceOnce) || !slices.Contains(v.Choices, surface.ChoiceCancel) {
			t.Fatalf("the Admin is offered %v", v.Choices)
		}
		if status, code := ann.decide(h.id, surface.ChoiceOnce); status != http.StatusOK {
			t.Fatalf("the Admin's approval answered %d %s", status, code)
		}
		if got := h.wait(t); got != "key=org" {
			t.Fatalf("the approved call answered %q", got)
		}
		if a := decidedBy(t, s, h.id); a.DecidedBy != "p-ann" {
			t.Fatalf("decided by %q", a.DecidedBy)
		}
		before = hold(t, bob, bobKey)
	})

	t.Run("only an Owner or Admin sets the credential mode", func(t *testing.T) {
		if status, _ := bob.b.send("PUT", "/api/connectors/"+conn+"/credential-mode", map[string]any{"mode": "per_person"}, nil); status != http.StatusForbidden {
			t.Fatalf("a Member's mode change answered %d", status)
		}
		if status, _ := owner.b.send("PUT", "/api/connectors/"+conn+"/credential-mode", map[string]any{"mode": "everyone"}, nil); status != http.StatusBadRequest {
			t.Fatalf("an unknown mode answered %d", status)
		}
		mode(console.CredentialPerPerson)
		if m := s.c.State.CredentialMode(conn); m != console.CredentialPerPerson {
			t.Fatalf("the mode is %s", m)
		}
	})

	t.Run("a call held before the switch keeps its approvers", func(t *testing.T) {
		if status, code := ann.decide(before.id, surface.ChoiceOnce); status != http.StatusOK {
			t.Fatalf("the Admin's approval of an older call answered %d %s", status, code)
		}
		before.wait(t)
		if a := decidedBy(t, s, before.id); a.DecidedBy != "p-ann" {
			t.Fatalf("decided by %q", a.DecidedBy)
		}
	})

	t.Run("each person signs in with their own credential", func(t *testing.T) {
		if got := callVault(t, bobKey, "whoami"); !strings.Contains(got, "credential_required") {
			t.Fatalf("a call before sign-in answered %q", got)
		}
		if b := bob.summary(); b.SignedIn || len(b.Credentials) != 0 {
			t.Fatalf("before sign-in the Member sees %+v", b)
		}
		bob.b.ok("POST", "/api/connectors/"+conn+"/headers", map[string]any{"headers": map[string]string{"X-Api-Key": "bob"}}, nil)
		owner.b.ok("POST", "/api/connectors/"+conn+"/headers", map[string]any{"headers": map[string]string{"X-Api-Key": "me"}}, nil)
		if got := callVault(t, bobKey, "whoami"); got != "key=bob" {
			t.Fatalf("the Member's call used %q", got)
		}
		if got := callVault(t, ownerKey, "whoami"); got != "key=me" {
			t.Fatalf("the Owner's call used %q", got)
		}
		if b := bob.summary(); !b.SignedIn || b.People != nil {
			t.Fatalf("after sign-in the Member sees %+v", b)
		}
		if o := owner.summary(); !o.SignedIn || *o.SignedInN != 2 || *o.People != 3 {
			t.Fatalf("after two sign-ins the Owner sees %+v", o)
		}
		if a := ann.summary(); a.SignedIn {
			t.Fatalf("the Admin who never signed in sees %+v", a)
		}
	})

	t.Run("only the key's owner approves a call on their own credential", func(t *testing.T) {
		h := hold(t, bob, bobKey)
		if v := owner.approval(h.id); !slices.Equal(v.Choices, []surface.Choice{surface.ChoiceCancel}) {
			t.Fatalf("the Owner is offered %v", v.Choices)
		}
		if v := bob.approval(h.id); slices.Contains(v.Choices, surface.ChoiceCancel) || !slices.Contains(v.Choices, surface.ChoiceOnce) {
			t.Fatalf("the key's owner is offered %v", v.Choices)
		}
		if status, code := owner.decide(h.id, surface.ChoiceOnce); status != http.StatusForbidden || code != "not_an_approver" {
			t.Fatalf("the Owner's approval answered %d %s", status, code)
		}
		if status, code := bob.decide(h.id, surface.ChoiceOnce); status != http.StatusOK {
			t.Fatalf("the key owner's approval answered %d %s", status, code)
		}
		if got := h.wait(t); got != "key=bob" {
			t.Fatalf("the approved call answered %q", got)
		}
	})

	t.Run("an Owner cancels a Member's waiting call", func(t *testing.T) {
		h := hold(t, bob, bobKey)
		if status, _ := bob.b.send("POST", "/api/approvals/"+h.id+"/cancel", nil, nil); status != http.StatusForbidden {
			t.Fatalf("a Member's cancel answered %d", status)
		}
		owner.b.ok("POST", "/api/approvals/"+h.id+"/cancel", nil, nil)
		h.wait(t)
		if a := decidedBy(t, s, h.id); a.State != port.ApprovalCancelled {
			t.Fatalf("the call is %s", a.State)
		}
	})

	t.Run("switching back uses the shared credential kept meanwhile", func(t *testing.T) {
		mode(console.CredentialShared)
		if got := callVault(t, bobKey, "whoami"); got != "key=org" {
			t.Fatalf("the Member's call used %q", got)
		}
		if o := owner.summary(); o.SignedIn || !slices.Equal(o.Credentials, []string{"custom_headers:active"}) {
			t.Fatalf("the shared connector shows %+v", o)
		}
	})
}

func TestASetupCarriesEachConnectorsCredentialMode(t *testing.T) {
	src := freshOwner(t)
	var imported struct{ Connector string }
	src.b.ok("POST", "/api/import", map[string]any{"mcp": newKeyEchoServer(t).URL + "/mcp", "local": true, "header_secret": true}, &imported)
	src.b.ok("PUT", "/api/connectors/"+imported.Connector+"/credential-mode", map[string]any{"mode": "per_person"}, nil)
	file := src.exportSetup()
	if !strings.Contains(string(file), "credentials: per_person") {
		t.Fatalf("the setup leaves out the mode:\n%s", file)
	}
	for _, tc := range []struct {
		name string
		file string
		want console.CredentialMode
	}{
		{"a setup with the field imports its mode", string(file), console.CredentialPerPerson},
		{"a setup without the field imports as shared", strings.ReplaceAll(string(file), "credentials: per_person", ""), console.CredentialShared},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dst := freshOwner(t)
			if status, raw := dst.upload("/api/backup/import", []byte(tc.file), ""); status != http.StatusOK || strings.Contains(string(raw), `"failed":{"`) {
				t.Fatalf("import: %d %s", status, raw)
			}
			var conns struct{ Connectors []console.ConnectorSummary }
			dst.b.ok("GET", "/api/connectors", nil, &conns)
			if len(conns.Connectors) != 1 || conns.Connectors[0].Mode != tc.want {
				t.Fatalf("connectors after the import %+v", conns.Connectors)
			}
		})
	}
	t.Run("an unknown mode is refused", func(t *testing.T) {
		dst := freshOwner(t)
		if status, _ := dst.upload("/api/backup/import", []byte(strings.ReplaceAll(string(file), "per_person", "everyone")), ""); status != http.StatusBadRequest {
			t.Fatalf("an unknown mode answered %d", status)
		}
	})
}
