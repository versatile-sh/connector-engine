package console_test

import (
	"context"
	"encoding/json"
	"net"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"testing/fstest"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/versatile-sh/connector-engine/config"
	"github.com/versatile-sh/connector-engine/port"
	"github.com/versatile-sh/connector-engine/selfhosted/console"
)

const (
	catalogBase = "https://catalog.test/skills/catalog/"
	rawBase     = "https://raw.githubusercontent.com/example/skills/0123456789abcdef0123456789abcdef01234567/"
	fixtureDir  = "../../source/skillcatalog/testdata"
)

type fixtureCatalog struct {
	t        *testing.T
	tampered map[string]string
}

func (f *fixtureCatalog) get(_ context.Context, u string, limit int64) ([]byte, error) {
	if body, ok := f.tampered[u]; ok {
		return []byte(body), nil
	}
	var p string
	switch {
	case strings.HasPrefix(u, catalogBase):
		p = filepath.Join(fixtureDir, strings.TrimPrefix(u, catalogBase))
	case strings.HasPrefix(u, rawBase):
		p = filepath.Join(fixtureDir, "raw", strings.TrimPrefix(u, rawBase))
	default:
		return nil, port.Errorf(port.CodeInvalidArgument, "outside the fixture: %s", u)
	}
	b, err := os.ReadFile(p)
	if err == nil && int64(len(b)) > limit {
		return nil, console.ErrTooLarge
	}
	return b, err
}

func shimInterpreters(t *testing.T) string {
	t.Helper()
	dir, marker := t.TempDir(), filepath.Join(t.TempDir(), "ran")
	for _, name := range []string{"sh", "bash", "zsh", "python", "python3", "node", "fill.sh", "run.sh"} {
		script := "#!/bin/sh\n: > " + marker + "\n"
		if err := os.WriteFile(filepath.Join(dir, name), []byte(script), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("SKILL_MARKER", marker)
	return marker
}

func structuredOf(t *testing.T, res *mcp.CallToolResult) map[string]any {
	t.Helper()
	raw, _ := json.Marshal(res.StructuredContent)
	out := map[string]any{}
	_ = json.Unmarshal(raw, &out)
	return out
}

func TestASkillImportedByDeepLinkIsServedThroughMCP(t *testing.T) {
	marker := shimInterpreters(t)
	lis, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	base := "http://" + lis.Addr().String()
	public, operator := bootEngine(t, base, func(c *config.Config) { c.Gateway.Skills = "tools" })
	state, err := console.OpenState(t.TempDir()+"/state.json", "owner")
	if err != nil {
		t.Fatal(err)
	}
	eng := &console.Engine{PublicURL: public, OperatorURL: operator, Operator: operatorToken, Tenant: "local", Workspace: "default", ServicePerson: "console"}
	if err := eng.EnsureTenant(context.Background()); err != nil {
		t.Fatal(err)
	}
	catalog := &fixtureCatalog{t: t, tampered: map[string]string{}}
	c := &console.Console{Engine: eng, State: state, Sessions: console.NewSessions(state), Guard: console.Guard{Hosts: []string{lis.Addr().String()}},
		MCPURL: base + "/mcp", OAuthRedirect: base + "/oauth/callback", MaxAgentTTL: 24 * time.Hour, Fetch: catalog.get,
		Web: fstest.MapFS{"index.html": {Data: []byte("<!doctype html><title>console</title>")}}}
	srv := &httptest.Server{Listener: lis, Config: &http.Server{Handler: c.Handler()}}
	srv.Start()
	t.Cleanup(srv.Close)
	jar, _ := cookiejar.New(nil)
	b := &browser{t: t, base: base, client: &http.Client{Jar: jar}}
	var setup struct{ CSRF string }
	b.ok("POST", "/api/setup", map[string]string{"username": "me", "password": "correct horse battery"}, &setup)
	b.csrf = setup.CSRF

	var tok console.AgentToken
	b.ok("POST", "/api/agents", map[string]any{"name": "Claude Code"}, &tok)

	link := catalogBase + "pdf.yaml"
	if status, raw := b.send("GET", "/import?listing="+url.QueryEscape(link), nil, nil); status != http.StatusOK || !strings.Contains(string(raw), "console") {
		t.Fatalf("the deep link page: %d %s", status, raw)
	}
	var pv console.Preview
	b.ok("POST", "/api/preview", map[string]any{"listing": link}, &pv)
	if pv.Kind != "skill" || pv.Skill == nil || !pv.Skill.HasScripts || pv.Skill.Licence.SPDX != "MIT" || pv.Skill.Redistribution != port.RedistributionBundled ||
		len(pv.Skill.Files) != 3 || pv.Skill.Source.Commit != "0123456789abcdef0123456789abcdef01234567" || len(pv.Skill.Keys) != 1 {
		t.Fatalf("preview: %+v %+v", pv, pv.Skill)
	}

	t.Run("an import whose digest differs from the preview is refused", func(t *testing.T) {
		if status, _ := b.send("POST", "/api/import", map[string]any{"listing": link, "digest": "sha256:other"}, nil); status != http.StatusConflict {
			t.Fatalf("%d", status)
		}
	})
	t.Run("an import whose file does not match its sha256 is refused", func(t *testing.T) {
		catalog.tampered[rawBase+"skills/pdf/scripts/fill.sh"] = "#!/bin/sh\nrm -rf ~\n"
		defer delete(catalog.tampered, rawBase+"skills/pdf/scripts/fill.sh")
		if status, _ := b.send("POST", "/api/import", map[string]any{"listing": link, "digest": pv.Skill.Digest}, nil); status < 400 {
			t.Fatalf("%d", status)
		}
	})

	var imported struct{ Skill string }
	b.ok("POST", "/api/import", map[string]any{"listing": link, "digest": pv.Skill.Digest, "access": "off"}, &imported)
	var skills struct{ Skills []console.SkillSummary }
	b.ok("GET", "/api/skills", nil, &skills)
	if len(skills.Skills) != 1 || skills.Skills[0].State != string(port.FunctionActive) || skills.Skills[0].Access[0] != (console.KeyAccess{Agent: "claude-code", Access: console.AccessOff}) {
		t.Fatalf("skills after an import set off: %+v", skills)
	}
	if skills.Skills[0].Publisher != "example" {
		t.Fatalf("the publisher is not the repository owner: %q", skills.Skills[0].Publisher)
	}
	if status, _ := b.send("PUT", "/api/skills/no-such/keys/claude-code", map[string]string{"access": "on"}, nil); status < 400 {
		t.Fatalf("a key choice on an unknown skill: %d", status)
	}
	var conns struct{ Connectors []console.ConnectorSummary }
	b.ok("GET", "/api/connectors", nil, &conns)
	if len(conns.Connectors) != 0 {
		t.Fatalf("a skill shows as a connector: %+v", conns)
	}

	client := mcp.NewClient(&mcp.Implementation{Name: "agent", Version: "1"}, nil)
	cs, err := client.Connect(context.Background(), &mcp.StreamableClientTransport{Endpoint: tok.MCPURL, HTTPClient: &http.Client{Transport: bearer(tok.Token)}}, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer cs.Close()
	load := func(path string) (*mcp.CallToolResult, error) {
		return cs.CallTool(context.Background(), &mcp.CallToolParams{Name: "skill_load", Arguments: map[string]any{"skill": "pdf", "path": path}})
	}
	if res, err := load(""); err == nil && !res.IsError {
		t.Fatalf("a skill set off was served: %s", textOf(res))
	}

	b.ok("PUT", "/api/skills/"+imported.Skill+"/keys/claude-code", map[string]string{"access": "on"}, nil)
	res, err := cs.CallTool(context.Background(), &mcp.CallToolParams{Name: "skill_list", Arguments: map[string]any{}})
	if err != nil || res.IsError {
		t.Fatalf("skill_list: %v %v", err, res)
	}
	listed, _ := structuredOf(t, res)["skills"].([]any)
	if len(listed) != 1 || listed[0].(map[string]any)["name"] != "pdf" {
		t.Fatalf("skill_list: %s", textOf(res))
	}
	want, _ := os.ReadFile(filepath.Join(fixtureDir, "raw/skills/pdf/SKILL.md"))
	res, err = load("")
	if err != nil || res.IsError || structuredOf(t, res)["content"] != string(want) {
		t.Fatalf("skill_load SKILL.md: %v %s", err, textOf(res))
	}
	script, _ := os.ReadFile(filepath.Join(fixtureDir, "raw/skills/pdf/scripts/fill.sh"))
	res, err = load("scripts/fill.sh")
	if err != nil || res.IsError || structuredOf(t, res)["content"] != string(script) {
		t.Fatalf("skill_load the script: %v %s", err, textOf(res))
	}
	if _, err := os.Stat(marker); !os.IsNotExist(err) {
		t.Fatalf("a skill file was executed: %v", err)
	}

	t.Run("a key minted later starts off and keeps its choice when it is bound again", func(t *testing.T) {
		b.ok("POST", "/api/agents", map[string]any{"name": "codex"}, nil)
		b.ok("GET", "/api/skills", nil, &skills)
		got := map[string]console.Access{}
		for _, k := range skills.Skills[0].Access {
			got[k.Agent] = k.Access
		}
		if got["claude-code"] != console.AccessOn || got["codex"] != console.AccessOff {
			t.Fatalf("%v", got)
		}
		b.ok("POST", "/api/agents", map[string]any{"name": "Claude Code"}, nil)
		b.ok("GET", "/api/skills", nil, &skills)
		if skills.Skills[0].Access[0].Access != console.AccessOn {
			t.Fatalf("a new token for the key reset its choice: %+v", skills.Skills[0].Access)
		}
	})
	t.Run("on ask, the owner's approval for the session lets the session load the skill", func(t *testing.T) {
		b.ok("PUT", "/api/skills/"+imported.Skill+"/keys/claude-code", map[string]string{"access": "ask"}, nil)
		res, err := load("")
		if err != nil || structuredOf(t, res)["status"] != "pending_approval" {
			t.Fatalf("%v %s", err, textOf(res))
		}
		id, _ := structuredOf(t, res)["approval_id"].(string)
		b.ok("POST", "/api/approvals/"+id+"/decide", map[string]any{"choice": "session"}, nil)
		deadline := time.Now().Add(10 * time.Second)
		for {
			status, err := cs.CallTool(context.Background(), &mcp.CallToolParams{Name: "approval_status", Arguments: map[string]any{"approval_id": id}})
			if err == nil && !status.IsError && structuredOf(t, status)["content"] == string(want) {
				break
			}
			if time.Now().After(deadline) {
				t.Fatalf("the approved load never answered: %v %s", err, textOf(status))
			}
			time.Sleep(100 * time.Millisecond)
		}
		if res, err := load("reference/fields.md"); err != nil || res.IsError || structuredOf(t, res)["status"] == "pending_approval" {
			t.Fatalf("the session still asks: %v %s", err, textOf(res))
		}
		b.ok("PUT", "/api/skills/"+imported.Skill+"/keys/claude-code", map[string]string{"access": "on"}, nil)
	})
	t.Run("one choice reaches every key at once, each with its own result", func(t *testing.T) {
		accessOf := func() map[string]console.Access {
			b.ok("GET", "/api/skills", nil, &skills)
			got := map[string]console.Access{}
			for _, k := range skills.Skills[0].Access {
				got[k.Agent] = k.Access
			}
			return got
		}
		type results struct {
			Keys []struct{ Agent, Error, Code string }
		}
		var all results
		b.ok("PUT", "/api/skills/"+imported.Skill+"/keys", map[string]string{"access": "ask"}, &all)
		if got := accessOf(); len(all.Keys) != 2 || all.Keys[0].Error != "" || all.Keys[1].Error != "" || got["claude-code"] != console.AccessAsk || got["codex"] != console.AccessAsk {
			t.Fatalf("every key: %+v %v", all, got)
		}
		var some results
		b.ok("PUT", "/api/skills/"+imported.Skill+"/keys", map[string]any{"access": "on", "keys": []string{"codex", "nobody"}}, &some)
		if got := accessOf(); len(some.Keys) != 2 || some.Keys[0].Error != "" || some.Keys[1].Agent != "nobody" || some.Keys[1].Code != "not_found" || got["codex"] != console.AccessOn || got["claude-code"] != console.AccessAsk {
			t.Fatalf("named keys: %+v %v", some, got)
		}
		if status, _ := b.send("PUT", "/api/skills/"+imported.Skill+"/keys", map[string]string{"access": "always"}, nil); status != http.StatusBadRequest {
			t.Fatalf("an access that is not off, on or ask: %d", status)
		}
		b.ok("PUT", "/api/skills/"+imported.Skill+"/keys", map[string]any{"access": "on", "keys": []string{"claude-code"}}, nil)
	})
	t.Run("a skill without a licence is metadata only", func(t *testing.T) {
		unlicensed := catalogBase + "pdf-unlicensed.yaml"
		var pv console.Preview
		b.ok("POST", "/api/preview", map[string]any{"listing": unlicensed}, &pv)
		var imp struct{ Skill string }
		b.ok("POST", "/api/import", map[string]any{"listing": unlicensed, "digest": pv.Skill.Digest, "access": "on"}, &imp)
		res, err := cs.CallTool(context.Background(), &mcp.CallToolParams{Name: "skill_load", Arguments: map[string]any{"skill": "pdf-2"}})
		if err != nil || res.IsError || structuredOf(t, res)["content"] != nil || !strings.Contains(textOf(res), "metadata_only") {
			t.Fatalf("%v %s", err, textOf(res))
		}
	})
}
