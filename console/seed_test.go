package console_test

import (
	"context"
	"crypto/rand"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/agents-control/connector-engine/config"
	"github.com/agents-control/connector-engine/console"
)

var (
	seedKeys       = []string{"Claude Code", "Cursor", "Codex", "VS Code", "OpenCode", "Antigravity", "Pi", "Zed", "Windsurf", "Cline", "Goose", "Continue"}
	seedConnectors = []string{"Notes", "Calendar", "Mail", "Drive", "Tickets", "Wiki", "Chat", "Billing", "Analytics", "Storage", "Deploys", "Logs", "Contacts", "Tasks", "Search"}
	seedSkills     = []string{"pdf-forms", "slides", "spreadsheets", "brand-voice", "release-notes", "code-review", "sql-helper", "meeting-notes", "changelog", "api-docs"}
)

func seedServer(t *testing.T, title string) string {
	name := strings.ToLower(title)
	srv := mcp.NewServer(&mcp.Implementation{Name: name, Title: title, Version: "1.0.0", Description: title + " for the seeded console"}, nil)
	yes, no := true, false
	for _, spec := range []struct {
		verb string
		ann  *mcp.ToolAnnotations
	}{{"list", &mcp.ToolAnnotations{ReadOnlyHint: true}}, {"search", &mcp.ToolAnnotations{ReadOnlyHint: true}}, {"create", &mcp.ToolAnnotations{DestructiveHint: &no}}, {"update", &mcp.ToolAnnotations{DestructiveHint: &no}}, {"delete", &mcp.ToolAnnotations{DestructiveHint: &yes}}} {
		srv.AddTool(&mcp.Tool{Name: spec.verb + "_" + name, Description: fmt.Sprintf("%s in %s", spec.verb, title), Annotations: spec.ann, InputSchema: map[string]any{"type": "object"}},
			func(context.Context, *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
				return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: "seeded"}}}, nil
			})
	}
	up := httptest.NewServer(mcp.NewStreamableHTTPHandler(func(*http.Request) *mcp.Server { return srv }, nil))
	t.Cleanup(up.Close)
	return up.URL
}

func seedListing(catalog *fixtureCatalog) func(context.Context, string, int64) ([]byte, error) {
	return func(ctx context.Context, u string, limit int64) ([]byte, error) {
		name, ok := strings.CutPrefix(u, catalogBase+"seed/")
		if !ok {
			return catalog.get(ctx, u, limit)
		}
		body, err := os.ReadFile(filepath.Join(fixtureDir, "pdf.yaml"))
		name = strings.TrimSuffix(name, ".yaml")
		title := strings.ToUpper(name[:1]) + strings.ReplaceAll(name[1:], "-", " ")
		return []byte(strings.Replace(string(body), "id: pdf\nname: pdf\ntitle: PDF forms\n", "id: "+name+"\nname: "+name+"\ntitle: "+title+"\n", 1)), err
	}
}

// TestSeedPlayground serves a console on CN_SEED_ADDR with 12 access keys, 15 connectors and 10 skills until interrupted, to see the layout at scale; only `make seed` runs it, and its state lives in a temporary folder.
func TestSeedPlayground(t *testing.T) {
	addr := os.Getenv("CN_SEED_ADDR")
	if addr == "" {
		t.Skip("make seed runs this playground")
	}
	slog.SetDefault(slog.New(slog.DiscardHandler))
	lis, err := net.Listen("tcp", addr)
	if err != nil {
		t.Fatal(err)
	}
	base := "http://" + lis.Addr().String()
	public, operator := bootEngine(t, base, func(c *config.Config) { c.Gateway.Skills = "tools" })
	state, err := console.OpenState(t.TempDir() + "/state.json")
	if err != nil {
		t.Fatal(err)
	}
	eng := &console.Engine{PublicURL: public, OperatorURL: operator, Operator: operatorToken, Tenant: "local", Workspace: "default", Person: "owner"}
	if err := eng.EnsureTenant(context.Background()); err != nil {
		t.Fatal(err)
	}
	c := &console.Console{Engine: eng, State: state, Sessions: console.NewSessions(state), Guard: console.Guard{Hosts: []string{lis.Addr().String()}},
		MCPURL: base + "/mcp", OAuthRedirect: base + "/oauth/callback", MaxAgentTTL: 720 * time.Hour, Fetch: seedListing(&fixtureCatalog{t: t, tampered: map[string]string{}}),
		Web: os.DirFS(filepath.Join("web", "dist", "app"))}
	srv := &httptest.Server{Listener: lis, Config: &http.Server{Handler: c.Handler()}}
	srv.Start()
	t.Cleanup(srv.Close)
	jar, _ := cookiejar.New(nil)
	b := &browser{t: t, base: base, client: &http.Client{Jar: jar}}
	var setup struct{ CSRF string }
	password := rand.Text()
	b.ok("POST", "/api/setup", map[string]string{"username": "owner", "password": password}, &setup)
	b.csrf = setup.CSRF
	fmt.Printf("Seeding %d keys, %d connectors and %d skills; each import binds every key, so this takes a few minutes.\n", len(seedKeys), len(seedConnectors), len(seedSkills))
	for _, name := range seedKeys {
		b.ok("POST", "/api/agents", map[string]any{"name": name, "ttl_hours": 720}, nil)
	}
	for i, title := range seedConnectors {
		b.ok("POST", "/api/import", map[string]any{"mcp": seedServer(t, title) + "/mcp", "local": true}, nil)
		fmt.Printf("connector %d/%d: %s\n", i+1, len(seedConnectors), title)
	}
	for i, name := range seedSkills {
		link := catalogBase + "seed/" + name + ".yaml"
		var pv console.Preview
		b.ok("POST", "/api/preview", map[string]any{"listing": link}, &pv)
		b.ok("POST", "/api/import", map[string]any{"listing": link, "digest": pv.Skill.Digest, "access": []string{"off", "on", "ask"}[i%3]}, nil)
		fmt.Printf("skill %d/%d: %s\n", i+1, len(seedSkills), name)
	}
	fmt.Printf("\nThe seeded console is at %s\nSign in as owner with the password %s (shown only here)\nCtrl-C stops it and forgets everything.\n\n", base, password)
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	<-ctx.Done()
}
