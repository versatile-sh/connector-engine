package remote_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/versatile-sh/connector-engine/core/pinning"
	"github.com/versatile-sh/connector-engine/credential"
	"github.com/versatile-sh/connector-engine/internal/egress"
	"github.com/versatile-sh/connector-engine/port"
	"github.com/versatile-sh/connector-engine/runtime/remote"
)

type upstream struct {
	*httptest.Server
	mu   sync.Mutex
	auth []string
}

func (u *upstream) authorizations() []string {
	u.mu.Lock()
	defer u.mu.Unlock()
	return append([]string(nil), u.auth...)
}

func newUpstream(t *testing.T, opts *mcp.StreamableHTTPOptions) *upstream {
	t.Helper()
	srv := mcp.NewServer(&mcp.Implementation{Name: "upstream", Version: "1"}, &mcp.ServerOptions{Instructions: "use with care"})
	schema := map[string]any{"type": "object"}
	no := false
	for _, tool := range []*mcp.Tool{
		{Name: "plain", InputSchema: schema},
		{Name: "flagged", InputSchema: schema, Annotations: &mcp.ToolAnnotations{DestructiveHint: &no}},
	} {
		srv.AddTool(tool, func(_ context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			return &mcp.CallToolResult{
				Content:           []mcp.Content{&mcp.TextContent{Text: "called " + req.Params.Name}},
				StructuredContent: map[string]any{"args": json.RawMessage(req.Params.Arguments)},
			}, nil
		})
	}
	srv.AddTool(&mcp.Tool{Name: "fails", InputSchema: schema}, func(context.Context, *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		return &mcp.CallToolResult{IsError: true, Content: []mcp.Content{&mcp.TextContent{Text: "boom"}}}, nil
	})
	handler := mcp.NewStreamableHTTPHandler(func(*http.Request) *mcp.Server { return srv }, opts)
	u := &upstream{}
	u.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		u.mu.Lock()
		u.auth = append(u.auth, r.Header.Get("Authorization"))
		u.mu.Unlock()
		handler.ServeHTTP(w, r)
	}))
	t.Cleanup(u.Close)
	return u
}

func runtimeFor(base string) (*remote.Runtime, port.Manifest) {
	applier := credential.Applier{
		Resolve: func(context.Context, port.SecretsRef) (port.CredentialKind, []byte, error) {
			return port.CredentialAPIKey, []byte("upstream-key"), nil
		},
		Injector: credential.Injector{},
	}
	return remote.New(applier, nil, egress.Guard{AllowPrivate: true}), port.Manifest{Kind: port.KindMCPRemote, Remote: &port.Remote{URL: base, Transport: "streamable-http"}}
}

func TestBothProtocolEras(t *testing.T) {
	for _, tc := range []struct {
		name string
		opts *mcp.StreamableHTTPOptions
		era  string
	}{
		{"session era over SSE", nil, "2025-11-25"},
		{"session era over JSON", &mcp.StreamableHTTPOptions{JSONResponse: true}, "2025-11-25"},
		{"stateless era", &mcp.StreamableHTTPOptions{Stateless: true, JSONResponse: true}, "2026-07-28"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			up := newUpstream(t, tc.opts)
			rt, m := runtimeFor(up.URL)
			ctx := context.Background()
			h, err := rt.Start(ctx, port.Connector{}, m, port.SecretsRef{Credential: "c1"})
			if err != nil {
				t.Fatal(err)
			}
			defer rt.Stop(ctx, h)

			defs, meta, err := rt.ListFunctions(ctx, h)
			if err != nil {
				t.Fatal(err)
			}
			if meta.ProtocolVersion != tc.era || meta.Instructions != "use with care" {
				t.Fatalf("meta %+v", meta)
			}
			if len(defs) != 3 {
				t.Fatalf("%d definitions", len(defs))
			}
			byName := map[string]port.RawDefinition{}
			for _, d := range defs {
				byName[d.Name] = d
			}
			if bytes.Contains(byName["plain"].Raw, []byte("annotations")) || !bytes.Contains(byName["flagged"].Raw, []byte(`"destructiveHint":false`)) {
				t.Fatalf("raw bytes lost the difference: %s / %s", byName["plain"].Raw, byName["flagged"].Raw)
			}
			plain, _ := pinning.DefinitionHash("plain", byName["plain"].Raw)
			flagged, _ := pinning.DefinitionHash("flagged", byName["flagged"].Raw)
			if plain == flagged {
				t.Fatal("an absent hint and an explicit false hash alike")
			}
			again, _, err := rt.ListFunctions(ctx, h)
			if err != nil || len(again) != 3 {
				t.Fatalf("second list: %v %d", err, len(again))
			}

			res, err := rt.Call(ctx, h, "plain", json.RawMessage(`{"x":1}`), time.Now().Add(5*time.Second))
			if err != nil || res.IsError {
				t.Fatalf("call: %v %+v", err, res)
			}
			if !bytes.Contains(res.Content, []byte("called plain")) || !bytes.Contains(res.Structured, []byte(`"x":1`)) {
				t.Fatalf("result %s | %s", res.Content, res.Structured)
			}
			if bad, err := rt.Call(ctx, h, "fails", nil, time.Time{}); err != nil || !bad.IsError {
				t.Fatalf("an upstream tool error is a result: %v %+v", err, bad)
			}
			for _, a := range up.authorizations() {
				if a != "Bearer upstream-key" {
					t.Fatalf("upstream saw Authorization %q", a)
				}
			}
		})
	}
}

func TestStartRefusesWhatItDoesNotServe(t *testing.T) {
	rt, _ := runtimeFor("http://127.0.0.1:1")
	for name, m := range map[string]port.Manifest{
		"wrong kind":    {Kind: port.KindRESTDeclarative},
		"sse transport": {Kind: port.KindMCPRemote, Remote: &port.Remote{URL: "http://127.0.0.1:1", Transport: "sse"}},
	} {
		if _, err := rt.Start(context.Background(), port.Connector{}, m, port.SecretsRef{}); !port.IsCode(err, port.CodeInvalidManifest) {
			t.Fatalf("%s: %v", name, err)
		}
	}
}

func TestStartRefusesNonPublicAddressesUnlessAllowed(t *testing.T) {
	up := newUpstream(t, &mcp.StreamableHTTPOptions{JSONResponse: true})
	_, m := runtimeFor(up.URL)
	ctx := context.Background()
	if _, err := remote.New(credential.Applier{}, nil, egress.Guard{}).Start(ctx, port.Connector{}, m, port.SecretsRef{}); !port.IsCode(err, port.CodeUnavailable) {
		t.Fatalf("loopback must be refused by default: %v", err)
	}
	host := strings.TrimPrefix(up.URL, "http://")
	m.Egress = &port.Egress{AllowHosts: []string{host}}
	if _, err := remote.New(credential.Applier{}, nil, egress.Guard{}).Start(ctx, port.Connector{}, m, port.SecretsRef{}); !port.IsCode(err, port.CodeUnavailable) {
		t.Fatalf("a private host named only in egress.allow_hosts must be refused: %v", err)
	}
	if _, err := remote.New(credential.Applier{}, nil, egress.Guard{Hosts: []string{"127.0.0.1"}}).Start(ctx, port.Connector{}, m, port.SecretsRef{}); err != nil {
		t.Fatalf("an operator-listed host is dialled: %v", err)
	}
}
