package mcpapi_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/agents-control/connector-engine/server"
	"github.com/agents-control/connector-engine/server/internal/harness"
	"github.com/agents-control/connector-engine/server/mcpapi"
)

type bearer struct{ token string }

func (b bearer) RoundTrip(r *http.Request) (*http.Response, error) {
	r = r.Clone(r.Context())
	r.Header.Set("Authorization", "Bearer "+b.token)
	return http.DefaultTransport.RoundTrip(r)
}

func connect(t *testing.T, h *harness.Harness, mutations []string, token string) (*mcp.ClientSession, error) {
	t.Helper()
	handler, err := mcpapi.New(mcpapi.Options{Env: h.Server, Authn: h.Authn, Mutations: mutations, Version: "test"})
	if err != nil {
		t.Fatal(err)
	}
	web := httptest.NewServer(handler)
	t.Cleanup(web.Close)
	client := mcp.NewClient(&mcp.Implementation{Name: "test", Version: "1"}, nil)
	session, err := client.Connect(context.Background(), &mcp.StreamableClientTransport{Endpoint: web.URL, HTTPClient: &http.Client{Transport: bearer{token}}}, nil)
	if err == nil {
		t.Cleanup(func() { _ = session.Close() })
	}
	return session, err
}

func names(t *testing.T, s *mcp.ClientSession) []string {
	t.Helper()
	list, err := s.ListTools(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	var out []string
	for _, tool := range list.Tools {
		out = append(out, tool.Name)
	}
	return out
}

func TestByDefaultOnlyReadingOperationsAreOffered(t *testing.T) {
	h := harness.New(t)
	s, err := connect(t, h, nil, h.AdminToken(t))
	if err != nil {
		t.Fatal(err)
	}
	got := names(t, s)
	for _, want := range []string{"listConnectors", "getApproval", "explainAccess", "lintListing", "listEventTypes"} {
		if !slices.Contains(got, want) {
			t.Errorf("%s is not offered", want)
		}
	}
	for _, o := range server.Operations() {
		if slices.Contains(got, o.ID) != (o.Reads() && !o.Live) {
			t.Errorf("%s offered: %v, reads: %v", o.ID, slices.Contains(got, o.ID), o.Reads())
		}
	}
	res, err := s.CallTool(context.Background(), &mcp.CallToolParams{Name: "putRole", Arguments: map[string]any{"person": "x"}})
	if err == nil && !res.IsError {
		t.Fatal("a mutation ran on the default surface")
	}
	res, err = s.CallTool(context.Background(), &mcp.CallToolParams{Name: "listRoles", Arguments: map[string]any{"tenant": "t1"}})
	if err != nil || res.IsError {
		t.Fatalf("a read failed: %v %+v", err, res)
	}
}

func TestANamedMutationIsOfferedAndTheDecisionOnesNever(t *testing.T) {
	h := harness.New(t)
	s, err := connect(t, h, []string{"putRole"}, h.AdminToken(t))
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Contains(names(t, s), "putRole") {
		t.Fatal("a named mutation is not offered")
	}
	res, err := s.CallTool(context.Background(), &mcp.CallToolParams{Name: "putRole", Arguments: map[string]any{"tenant": "t1", "person": "bia", "role": "approver", "scope": "t/t1"}})
	if err != nil || res.IsError {
		t.Fatalf("%v %+v", err, res)
	}
	for _, never := range []string{"decideApproval", "cancelApproval", "clearPause"} {
		if _, err := mcpapi.New(mcpapi.Options{Env: h.Server, Authn: h.Authn, Mutations: []string{never}}); err == nil {
			t.Errorf("%s was accepted in manage.mcp.operations", never)
		}
		if slices.Contains(names(t, s), never) {
			t.Errorf("%s is offered", never)
		}
	}
	if _, err := mcpapi.New(mcpapi.Options{Env: h.Server, Authn: h.Authn, Mutations: []string{"nope"}}); err == nil {
		t.Error("an unknown operation was accepted")
	}
}

func TestAnAgentTokenCannotConnect(t *testing.T) {
	h := harness.New(t)
	if _, err := connect(t, h, nil, h.AgentToken(t)); err == nil {
		t.Fatal("an agent principal reached the management surface")
	}
	if _, err := connect(t, h, nil, h.AgentToken(t, "https://agent.test")); err == nil {
		t.Fatal("a per-run token of the agent audience reached the management surface")
	}
}

func TestTheRefusalNamesTheCodeBeforeAnyAuthorization(t *testing.T) {
	h := harness.New(t)
	handler, err := mcpapi.New(mcpapi.Options{Env: h.Server, Authn: h.Authn})
	if err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodPost, "/", nil)
	req.Header.Set("Authorization", "Bearer "+h.AgentToken(t))
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden || !strings.Contains(rec.Body.String(), `"agent_principal_refused"`) {
		t.Fatalf("%d %s", rec.Code, rec.Body)
	}
}
