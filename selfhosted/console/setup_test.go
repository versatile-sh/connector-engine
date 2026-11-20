package console_test

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"slices"
	"strings"
	"testing"
	"time"

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

func TestASharedSetupCarriesEachKeyProfile(t *testing.T) {
	src := profiledStack(t)
	for _, name := range []string{"alpha", "beta", "gamma"} {
		src.mint(name)
	}
	src.narrow("alpha", "lookup", console.AccessOff)
	src.mode(console.ProfileOff, "beta")
	file := src.exportSetup()
	if strings.Contains(string(file), "gamma") || strings.Contains(string(file), "newKeysOff") {
		t.Fatalf("the setup:\n%s", file)
	}

	dst := profiled{stack: freshOwner(t, personRelays)}
	beta := dst.mint("beta")
	var res console.SetupResult
	status, raw := dst.upload("/api/backup/import", file, "")
	if err := json.Unmarshal(raw, &res); status != http.StatusOK || err != nil || len(res.Failed) != 0 {
		t.Fatalf("import: %d %s", status, raw)
	}
	var conns struct{ Connectors []console.ConnectorSummary }
	dst.b.ok("GET", "/api/connectors", nil, &conns)
	dst.conn = conns.Connectors[0].ID
	alpha, delta := dst.mint("alpha"), dst.mint("delta")
	got := map[string]console.Profile{}
	for _, p := range dst.summary().Profiles {
		got[p.Agent] = p
	}
	if p := got["alpha"]; p.Mode != console.ProfileCustom || p.Overrides["lookup"] != console.AccessOff {
		t.Errorf("alpha, made after the import: %+v", p)
	}
	if got["beta"].Mode != console.ProfileOff || got["delta"].Mode != console.ProfileFollow {
		t.Errorf("beta, made before the import, and a new key: %+v", got)
	}
	if got := toolsOf(t, alpha); !slices.Equal(got, []string{"file"}) {
		t.Errorf("alpha lists %v", got)
	}
	if len(toolsOf(t, beta)) != 0 || len(toolsOf(t, delta)) != 2 {
		t.Error("beta lists the connector's tools, or delta does not")
	}
}

func (p *profiled) importSetup(file []byte) {
	p.t.Helper()
	var res console.SetupResult
	status, raw := p.upload("/api/backup/import", file, "")
	if err := json.Unmarshal(raw, &res); status != http.StatusOK || err != nil || len(res.Failed) != 0 {
		p.t.Fatalf("import: %d %s", status, raw)
	}
	var conns struct{ Connectors []console.ConnectorSummary }
	p.b.ok("GET", "/api/connectors", nil, &conns)
	p.conn = conns.Connectors[0].ID
}

func TestNewKeysStartOffEvenWithAWaitingProfile(t *testing.T) {
	src := profiledStack(t)
	src.mint("alpha")
	src.narrow("alpha", "lookup", console.AccessOff)
	plain := src.exportSetup()
	src.b.ok("PUT", "/api/connectors/"+src.conn+"/new-keys", map[string]bool{"off": true}, nil)
	startOff := src.exportSetup()
	if !strings.Contains(string(startOff), "newKeysOff: true") {
		t.Fatalf("the setup:\n%s", startOff)
	}

	t.Run("a setup whose new keys start off leaves no profile waiting", func(t *testing.T) {
		dst := profiled{stack: freshOwner(t, personRelays)}
		dst.importSetup(startOff)
		if w := dst.summary().Waiting; len(w) != 0 {
			t.Fatalf("waiting %+v", w)
		}
		if p := dst.profile(dst.mint("alpha").Agent); p.Mode != console.ProfileOff {
			t.Fatalf("alpha: %+v", p)
		}
	})

	t.Run("making new keys start off drops the profiles waiting there", func(t *testing.T) {
		dst := profiled{stack: freshOwner(t, personRelays)}
		dst.importSetup(plain)
		if w := dst.summary().Waiting; len(w) != 1 || w[0].Agent != "alpha" || w[0].Mode != console.ProfileCustom {
			t.Fatalf("waiting %+v", w)
		}
		dst.b.ok("PUT", "/api/connectors/"+dst.conn+"/new-keys", map[string]bool{"off": true}, nil)
		if w := dst.summary().Waiting; len(w) != 0 {
			t.Fatalf("still waiting %+v", w)
		}
		if p := dst.profile(dst.mint("alpha").Agent); p.Mode != console.ProfileOff {
			t.Fatalf("alpha: %+v", p)
		}
	})

	t.Run("a waiting profile is cleared by hand", func(t *testing.T) {
		dst := profiled{stack: freshOwner(t, personRelays)}
		dst.importSetup(plain)
		dst.b.ok("DELETE", "/api/connectors/"+dst.conn+"/waiting/alpha", nil, nil)
		if w := dst.summary().Waiting; len(w) != 0 {
			t.Fatalf("still waiting %+v", w)
		}
		if p := dst.profile(dst.mint("alpha").Agent); p.Mode != console.ProfileFollow {
			t.Fatalf("alpha: %+v", p)
		}
	})

	t.Run("a profile names a key as a key name is written", func(t *testing.T) {
		dst := profiled{stack: freshOwner(t, personRelays)}
		dst.importSetup(bytes.Replace(plain, []byte("key: alpha"), []byte("key: Alpha"), 1))
		if p := dst.profile(dst.mint("alpha").Agent); p.Mode != console.ProfileCustom {
			t.Fatalf("alpha: %+v", p)
		}
	})

	t.Run("a setup refuses a profile that follows or asks on instructions", func(t *testing.T) {
		dst := freshOwner(t, personRelays)
		for name, file := range map[string][]byte{
			"follow":              bytes.Replace(plain, []byte("mode: custom"), []byte("mode: follow"), 1),
			"ask on instructions": bytes.Replace(plain, []byte("lookup: \"off\""), []byte(`"instructions:server": ask`), 1),
		} {
			if status, raw := dst.upload("/api/backup/inspect", file, ""); status != http.StatusBadRequest {
				t.Errorf("%s: %d %s\n%s", name, status, raw, file)
			}
		}
	})
}

func TestAWaitingProfileNeverStopsItsKeyBeingMade(t *testing.T) {
	src := profiledStack(t)
	src.mint("beta")
	src.narrow("beta", "lookup", console.AccessAsk)
	src.b.ok("PUT", "/api/connectors/"+src.conn+"/functions/lookup", map[string]string{"access": "off"}, nil)
	file := src.exportSetup()

	dst := profiled{stack: freshOwner(t, personRelays)}
	if status, raw := dst.upload("/api/backup/import", file, ""); status != http.StatusOK {
		t.Fatalf("import: %d %s", status, raw)
	}
	var conns struct{ Connectors []console.ConnectorSummary }
	dst.b.ok("GET", "/api/connectors", nil, &conns)
	dst.conn = conns.Connectors[0].ID
	beta := dst.mint("beta")
	if p := dst.profile("beta"); p.Mode != console.ProfileCustom || p.Overrides["lookup"] != console.AccessAsk {
		t.Fatalf("beta keeps its dormant override: %+v", p)
	}
	if got := toolsOf(t, beta); !slices.Equal(got, []string{"file"}) {
		t.Fatalf("beta lists %v", got)
	}
}

type refuseOverrides struct{ next http.RoundTripper }

func (r refuseOverrides) RoundTrip(req *http.Request) (*http.Response, error) {
	if req.Method == http.MethodPost && req.URL.Path == "/v1/grants" && req.Body != nil {
		raw, _ := io.ReadAll(req.Body)
		req.Body = io.NopCloser(bytes.NewReader(raw))
		if bytes.Contains(raw, []byte(`"mode":"deny"`)) {
			return &http.Response{StatusCode: http.StatusInternalServerError, Body: io.NopCloser(strings.NewReader(`{"code":"internal","detail":"refused"}`)), Header: http.Header{}, Request: req}, nil
		}
	}
	return r.next.RoundTrip(req)
}

func TestAConnectorWhoseProfilesFailIsReportedOnce(t *testing.T) {
	src := profiledStack(t)
	src.mint("alpha")
	src.narrow("alpha", "lookup", console.AccessOff)
	file := src.exportSetup()

	dst := profiled{stack: freshOwner(t, personRelays)}
	dst.mint("alpha")
	dst.c.Engine.HTTP = &http.Client{Timeout: time.Minute, Transport: refuseOverrides{next: http.DefaultTransport}}
	var res console.SetupResult
	status, raw := dst.upload("/api/backup/import", file, "")
	if err := json.Unmarshal(raw, &res); status != http.StatusOK || err != nil {
		t.Fatalf("import: %d %s", status, raw)
	}
	if _, failed := res.Failed["notes"]; !failed || slices.Contains(res.Added, "notes") {
		t.Fatalf("result %+v", res)
	}
}

func TestAProfileThatFailsHalfwayLeavesTheKeyOff(t *testing.T) {
	src := profiledStack(t)
	src.mint("alpha")
	src.narrow("alpha", "lookup", console.AccessOff)
	file := src.exportSetup()

	t.Run("copying a profile", func(t *testing.T) {
		p := profiledStack(t)
		p.mint("beta")
		gamma := p.mint("gamma")
		p.narrow("beta", "lookup", console.AccessOff)
		p.mode(console.ProfileOff, "gamma")
		p.c.Engine.HTTP = &http.Client{Timeout: time.Minute, Transport: refuseOverrides{next: http.DefaultTransport}}
		if codes := p.batch("POST", "/api/connectors/"+p.conn+"/profiles:copy", map[string]any{"from": "beta", "keys": []string{"gamma"}}); codes["gamma"] == "" {
			t.Fatalf("copy: %v", codes)
		}
		p.c.Engine.HTTP = &http.Client{Timeout: time.Minute}
		if pr := p.profile("gamma"); pr.Mode != console.ProfileOff || len(toolsOf(t, gamma)) != 0 {
			t.Fatalf("gamma after a failed copy: %+v", pr)
		}
	})

	t.Run("a key made again after a failure still takes its profile", func(t *testing.T) {
		dst := profiled{stack: freshOwner(t, personRelays)}
		dst.importSetup(file)
		dst.c.Engine.HTTP = &http.Client{Timeout: time.Minute, Transport: refuseOverrides{next: http.DefaultTransport}}
		if status, raw := dst.b.send("POST", "/api/agents", map[string]any{"name": "alpha"}, nil); status < 500 {
			t.Fatalf("mint with overrides refused: %d %s", status, raw)
		}
		dst.c.Engine.HTTP = &http.Client{Timeout: time.Minute}
		alpha := dst.mint("alpha")
		if p := dst.profile("alpha"); p.Mode != console.ProfileCustom || p.Overrides["lookup"] != console.AccessOff {
			t.Fatalf("alpha after the retry: %+v", p)
		}
		if got := toolsOf(t, alpha); !slices.Equal(got, []string{"file"}) {
			t.Fatalf("alpha lists %v", got)
		}
	})
}
