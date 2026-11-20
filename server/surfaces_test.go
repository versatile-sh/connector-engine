package server_test

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/emptypb"

	connectorenginev1 "github.com/versatile-sh/connector-engine/api/connectorengine/v1"
	"github.com/versatile-sh/connector-engine/server"
	"github.com/versatile-sh/connector-engine/server/grpcapi"
	"github.com/versatile-sh/connector-engine/server/httpapi"
	"github.com/versatile-sh/connector-engine/server/internal/harness"
)

type surfaces struct {
	h    *harness.Harness
	url  string
	grpc connectorenginev1.ConnectorEngineClient
}

func newSurfaces(t *testing.T) *surfaces {
	t.Helper()
	h := harness.New(t)
	handler, err := httpapi.New(httpapi.Options{Env: h.Server, Authn: h.Authn})
	if err != nil {
		t.Fatal(err)
	}
	web := httptest.NewServer(handler)
	t.Cleanup(web.Close)
	srv, err := grpcapi.New(h.Server, h.Authn, nil)
	if err != nil {
		t.Fatal(err)
	}
	lis, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	go func() { _ = srv.Serve(lis) }()
	t.Cleanup(srv.Stop)
	conn, err := grpc.NewClient(lis.Addr().String(), grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	return &surfaces{h: h, url: web.URL, grpc: connectorenginev1.NewConnectorEngineClient(conn)}
}

func (s *surfaces) do(t *testing.T, method, path, token, key string, body any) (int, map[string]any, http.Header) {
	t.Helper()
	var in io.Reader
	if body != nil {
		data, _ := json.Marshal(body)
		in = bytes.NewReader(data)
	}
	req, _ := http.NewRequest(method, s.url+path, in)
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	if key != "" {
		req.Header.Set("Idempotency-Key", key)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	var out map[string]any
	_ = json.NewDecoder(resp.Body).Decode(&out)
	return resp.StatusCode, out, resp.Header
}

func (s *surfaces) ctx(token string) context.Context {
	return metadata.AppendToOutgoingContext(context.Background(), "authorization", "Bearer "+token)
}

func TestHTTPRolesRoundTripAndIdempotentRetry(t *testing.T) {
	s := newSurfaces(t)
	admin := s.h.AdminToken(t)
	role := map[string]any{"tenant": "t1", "person": "bia", "role": "approver", "scope": "t/t1/w/w1"}
	if code, _, _ := s.do(t, http.MethodPut, "/v1/roles", admin, "k1", role); code != http.StatusOK {
		t.Fatalf("put: %d", code)
	}
	role["role"] = "auditor"
	if code, _, _ := s.do(t, http.MethodPut, "/v1/roles", admin, "k1", role); code != http.StatusOK {
		t.Fatalf("retry: %d", code)
	}
	code, out, _ := s.do(t, http.MethodGet, "/v1/roles?tenant=t1&person=bia", admin, "", nil)
	items, _ := out["items"].([]any)
	if code != http.StatusOK || len(items) != 1 || items[0].(map[string]any)["role"] != "approver" {
		t.Fatalf("a retry with the same key wrote again: %d %v", code, out)
	}
	if code, _, _ := s.do(t, http.MethodDelete, "/v1/roles?tenant=t1&person=bia&role=approver&scope=t/t1/w/w1", admin, "", nil); code != http.StatusNoContent {
		t.Fatalf("delete: %d", code)
	}
}

func TestHTTPErrorsArriveAsProblemDocuments(t *testing.T) {
	s := newSurfaces(t)
	admin := s.h.AdminToken(t)
	for _, c := range []struct {
		name, method, path, token string
		status                    int
		code                      string
	}{
		{"no token", http.MethodGet, "/v1/roles", "", 401, "unauthenticated"},
		{"garbage token", http.MethodGet, "/v1/roles", "nope", 401, "unauthenticated"},
		{"another audience", http.MethodGet, "/v1/roles", s.h.AgentToken(t, "https://agent.test"), 401, "unauthenticated"},
		{"agent claim", http.MethodGet, "/v1/roles", s.h.AgentToken(t), 403, "agent_principal_refused"},
		{"unknown connector", http.MethodGet, "/v1/connectors/nope", admin, 404, "not_found"},
		{"no route", http.MethodGet, "/v1/nope", admin, 404, "not_found"},
		{"wrong method", http.MethodPatch, "/v1/roles", admin, 405, "method_not_allowed"},
		{"not an admin", http.MethodPut, "/v1/tenants/t9", admin, 403, "permission_denied"},
	} {
		code, out, header := s.do(t, c.method, c.path, c.token, "", nil)
		if code != c.status || out["code"] != c.code || header.Get("Content-Type") != server.ProblemMediaType {
			t.Errorf("%s: %d %v %s", c.name, code, out, header.Get("Content-Type"))
		}
	}
	code, out, _ := s.do(t, http.MethodPut, "/v1/roles", admin, "", map[string]any{"person": "x", "nope": 1})
	if code != http.StatusBadRequest || out["code"] != "invalid_argument" {
		t.Errorf("unknown body field: %d %v", code, out)
	}
}

func TestEventTypesArePublicAndCarryTheirSchemas(t *testing.T) {
	s := newSurfaces(t)
	code, out, _ := s.do(t, http.MethodGet, "/v1/event-types", "", "", nil)
	types, _ := out["types"].([]any)
	if code != http.StatusOK || len(types) != 43 {
		t.Fatalf("%d types, status %d", len(types), code)
	}
	code, out, _ = s.do(t, http.MethodGet, "/v1/event-types/connector.call.finished", "", "", nil)
	if schema, _ := out["schema"].(map[string]any); code != http.StatusOK || schema["title"] != "connector.call.finished" {
		t.Fatalf("%d %v", code, out)
	}
	if code, _, _ = s.do(t, http.MethodGet, "/v1/event-types/nope", "", "", nil); code != http.StatusNotFound {
		t.Fatalf("unknown type: %d", code)
	}
}

func TestGRPCAnswersWhatHTTPAnswers(t *testing.T) {
	s := newSurfaces(t)
	admin := s.h.AdminToken(t)
	ctx := metadata.AppendToOutgoingContext(s.ctx(admin), "idempotency-key", "k9")
	if _, err := s.grpc.PutRole(ctx, &connectorenginev1.RoleAssignment{Tenant: "t1", Person: "bia", Role: "approver", Scope: "t/t1/w/w1"}); err != nil {
		t.Fatal(err)
	}
	list, err := s.grpc.ListRoles(s.ctx(admin), &connectorenginev1.ListRolesRequest{Tenant: "t1", Person: "bia"})
	if err != nil || len(list.Items) != 1 || list.Items[0].Role != "approver" {
		t.Fatalf("%v %v", list, err)
	}
	_, out, _ := s.do(t, http.MethodGet, "/v1/roles?tenant=t1&person=bia", admin, "", nil)
	if items, _ := out["items"].([]any); len(items) != len(list.Items) {
		t.Fatalf("http %v, grpc %v", out, list)
	}
	types, err := s.grpc.ListEventTypes(context.Background(), &emptypb.Empty{})
	if err != nil || len(types.Types) != 43 {
		t.Fatalf("%v %v", types, err)
	}
	if _, err := s.grpc.GetConnector(s.ctx(admin), &connectorenginev1.ConnectorRequest{Id: "nope"}); status.Code(err) != codes.NotFound {
		t.Fatalf("not found: %v", err)
	}
	if _, err := s.grpc.ListRoles(context.Background(), &connectorenginev1.ListRolesRequest{}); status.Code(err) != codes.Unauthenticated {
		t.Fatalf("no token: %v", err)
	}
}
