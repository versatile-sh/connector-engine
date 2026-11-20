package console_test

import (
	"encoding/json"
	"net/http"
	"slices"
	"strings"
	"testing"

	"github.com/versatile-sh/connector-engine/config"
	"github.com/versatile-sh/connector-engine/selfhosted/console"
	storememory "github.com/versatile-sh/connector-engine/store/memory"
)

func (s *stack) exportSetup() []byte {
	s.t.Helper()
	status, raw := s.b.send("POST", "/api/backup/export", map[string]any{"mode": "setup"}, nil)
	if status != http.StatusOK {
		s.t.Fatalf("export setup: %d %s", status, raw)
	}
	return raw
}

func (s *stack) choices() map[string]map[string]console.Access {
	s.t.Helper()
	out := map[string]map[string]console.Access{}
	v := s.view()
	for _, c := range v.Connectors {
		m := map[string]console.Access{}
		for _, f := range c.Functions {
			m[f.Name] = f.Access
		}
		for _, it := range c.Items {
			m[it.Name] = it.Access
		}
		out[c.Alias] = m
	}
	for _, sk := range v.Skills {
		out["skill:"+sk.Name] = map[string]console.Access{}
	}
	return out
}

func freshOwner(t *testing.T, tweaks ...func(*config.Config)) *stack {
	t.Helper()
	s := openStack(t, t.TempDir(), storememory.New(), tweaks...)
	s.b.ok("POST", "/api/setup", map[string]string{"username": "me", "password": ownerPassword}, nil)
	s.signIn()
	return s
}

func seededSetup(t *testing.T) (*stack, console.AgentToken, string) {
	t.Helper()
	src, tok, _ := seeded(t)
	query := newToolServer(t, "maps", "route")
	src.b.ok("POST", "/api/import", map[string]any{"mcp": query.URL + "/mcp?api_key=Q-SECRET-42&region=eu", "local": true}, nil)
	var conns struct{ Connectors []console.ConnectorSummary }
	src.b.ok("GET", "/api/connectors", nil, &conns)
	for _, c := range conns.Connectors {
		if c.Alias == "notes" {
			src.b.ok("PUT", "/api/connectors/"+c.ID+"/functions/lookup", map[string]string{"access": "ask"}, nil)
		}
	}
	if err := src.c.State.PutChannel(console.StoredChannel{ID: "ch-1", Kind: "telegram", Label: "my phone", Settings: map[string]string{"chat_id": "42"}}); err != nil {
		t.Fatal(err)
	}
	return src, tok, query.URL
}

func TestASharedSetupHoldsNoSecret(t *testing.T) {
	src, tok, query := seededSetup(t)
	hidden := newToolServer(t, "hidden", "peek")
	src.b.ok("POST", "/api/import", map[string]any{"mcp": hidden.URL + "/mcp/sk-ak-0123456789abcdefghij/sse?k=K-SECRET-8", "local": true}, nil)
	out := src.exportSetup()
	for _, secret := range []string{apiKey, tok.Token, "Q-SECRET-42", "K-SECRET-8", "sk-ak-0123456789", operatorToken, ownerPassword, "a console secret", "client-1", "chat_id", "my phone", "api_key="} {
		if strings.Contains(string(out), secret) {
			t.Errorf("the setup holds %q:\n%s", secret, out)
		}
	}
	if strings.Contains(string(out), "region=eu") || !strings.Contains(string(out), "removed the query from an address of http://127.0.0.1:") {
		t.Errorf("the setup keeps a query or does not say it removed it:\n%s", out)
	}
	if !strings.Contains(string(out), "its path looks like it carries a secret") {
		t.Errorf("the setup does not say it left out an address with a secret path:\n%s", out)
	}
	if !strings.Contains(string(out), "telegram") || !strings.Contains(string(out), strings.TrimPrefix(query, "http://")) {
		t.Errorf("the setup lacks the channel kind or a source:\n%s", out)
	}
}

func TestASharedSetupInstallsTheSameChoicesOnAFreshInstall(t *testing.T) {
	src, _, _ := seededSetup(t)
	file := src.exportSetup()
	dst := freshOwner(t)
	var sum console.BackupSummary
	status, raw := dst.upload("/api/backup/inspect", file, "")
	if err := json.Unmarshal(raw, &sum); status != http.StatusOK || err != nil {
		t.Fatalf("inspect: %d %s", status, raw)
	}
	if sum.Kind != "setup" || sum.Setup == nil || len(sum.Setup.Connectors) != 3 || len(sum.Setup.Skills) != 1 || !slices.Equal(sum.Setup.Notifications, []string{"telegram"}) {
		t.Fatalf("summary %+v", sum)
	}
	var res console.SetupResult
	status, raw = dst.upload("/api/backup/import", file, "")
	if err := json.Unmarshal(raw, &res); status != http.StatusOK || err != nil {
		t.Fatalf("import: %d %s", status, raw)
	}
	if len(res.Added) != 4 || len(res.Failed) != 0 || len(res.SignIns) != 3 {
		t.Fatalf("result %+v", res)
	}
	want, got := src.choices(), dst.choices()
	wj, _ := json.Marshal(want)
	gj, _ := json.Marshal(got)
	if string(wj) != string(gj) {
		t.Fatalf("choices after the setup:\n got %s\nwant %s", gj, wj)
	}
}

func TestASharedSetupAddsToAnInstallWithoutChangingWhatIsThere(t *testing.T) {
	src, _, _ := seededSetup(t)
	file := src.exportSetup()
	var conns struct{ Connectors []console.ConnectorSummary }
	src.b.ok("GET", "/api/connectors", nil, &conns)
	var notes console.ConnectorSummary
	for _, c := range conns.Connectors {
		if c.Alias == "notes" {
			notes = c
		}
	}
	dst := freshOwner(t)
	dst.b.ok("POST", "/api/import", map[string]any{"mcp": notes.RemoteURL, "local": true, "choices": map[string]string{"lookup": "off"}}, nil)
	own := newToolServer(t, "mine", "keep")
	dst.b.ok("POST", "/api/import", map[string]any{"mcp": own.URL + "/mcp", "local": true}, nil)

	var res console.SetupResult
	status, raw := dst.upload("/api/backup/import", file, "")
	if err := json.Unmarshal(raw, &res); status != http.StatusOK || err != nil {
		t.Fatalf("import: %d %s", status, raw)
	}
	if !slices.Contains(res.Kept, "notes") || len(res.Added) != 3 || len(res.Failed) != 0 {
		t.Fatalf("result %+v", res)
	}
	got := dst.choices()
	if got["notes"]["lookup"] != console.AccessOff {
		t.Fatalf("the setup changed a choice already made: %v", got["notes"])
	}
	if _, ok := got["mine"]; !ok || len(got) != 5 {
		t.Fatalf("after the setup: %v", got)
	}
}
