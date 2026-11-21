package console_test

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/versatile-sh/connector-engine/selfhosted/console"
)

func slotSetup(t *testing.T) []byte {
	t.Helper()
	src := profiledStack(t)
	for _, name := range []string{"alpha", "beta", "gamma"} {
		src.mint(name)
	}
	src.narrow("alpha", "lookup", console.AccessAsk)
	src.mode(console.ProfileOff, "beta")
	return src.exportSetup()
}

func (p profiled) slots() map[string]console.SlotView {
	p.t.Helper()
	var out struct{ Slots []console.SlotView }
	p.b.ok("GET", "/api/slots", nil, &out)
	m := map[string]console.SlotView{}
	for _, s := range out.Slots {
		m[s.Name] = s
	}
	return m
}

func (p profiled) hasExactly(agent string, mode console.ProfileMode, overrides map[string]console.Access) {
	p.t.Helper()
	got := p.profile(agent)
	if got.Mode != mode || len(got.Overrides) != len(overrides) {
		p.t.Fatalf("%s: %+v, want %s %v", agent, got, mode, overrides)
	}
	for name, a := range overrides {
		if got.Overrides[name] != a {
			p.t.Fatalf("%s: %+v, want %s %v", agent, got, mode, overrides)
		}
	}
}

func importedSlots(t *testing.T, file []byte, before ...string) profiled {
	t.Helper()
	dst := profiled{stack: freshOwner(t, personRelays)}
	for _, name := range before {
		dst.mint(name)
	}
	dst.importSetup(file)
	return dst
}

func (p profiled) consent(slot string) string {
	p.t.Helper()
	return p.consentWith(p.register(), slot)
}

func (p profiled) register() string {
	p.t.Helper()
	raw, _ := json.Marshal(map[string]any{"client_name": "Claude Code", "redirect_uris": []string{"http://127.0.0.1:33418/callback"}})
	resp, err := http.Post(p.base+"/oauth/register", "application/json", bytes.NewReader(raw))
	if err != nil {
		p.t.Fatal(err)
	}
	defer resp.Body.Close()
	var reg struct {
		ClientID string `json:"client_id"`
	}
	_ = json.NewDecoder(resp.Body).Decode(&reg)
	return reg.ClientID
}

func (p profiled) consentWith(client, slot string) string {
	p.t.Helper()
	q := url.Values{"response_type": {"code"}, "client_id": {client}, "redirect_uri": {"http://127.0.0.1:33418/callback"}, "state": {"s1"},
		"code_challenge": {challengeOf(strings.Repeat("v", 50))}, "code_challenge_method": {"S256"}}
	noRedo := &http.Client{Jar: p.b.client.Jar, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	resp, err := noRedo.Get(p.base + "/oauth/authorize?" + q.Encode())
	if err != nil {
		p.t.Fatal(err)
	}
	resp.Body.Close()
	loc, _ := url.Parse(resp.Header.Get("Location"))
	if loc.Path != "/connect" {
		p.t.Fatalf("authorize: %d %s", resp.StatusCode, loc)
	}
	p.b.ok("POST", "/api/oauth/requests/"+loc.Query().Get("request"), map[string]any{"approve": true, "password": ownerPassword, "slot": slot}, nil)
	var keys struct {
		Agents []struct {
			Name     string `json:"name"`
			ClientID string `json:"client_id"`
		} `json:"agents"`
	}
	p.b.ok("GET", "/api/agents", nil, &keys)
	for _, a := range keys.Agents {
		if a.ClientID == client {
			return a.Name
		}
	}
	p.t.Fatalf("no key for client %s in %+v", client, keys.Agents)
	return ""
}

func TestASharedSetupBringsEachKeyAsAPermissionSetNeverBoundByName(t *testing.T) {
	file := slotSetup(t)
	dst := importedSlots(t, file, "alpha")
	var sum console.BackupSummary
	_, raw := freshOwner(t).upload("/api/backup/inspect", file, "")
	if err := json.Unmarshal(raw, &sum); err != nil || len(sum.Setup.Slots) != 3 || sum.Setup.Slots[0].Name != "alpha" || sum.Setup.Slots[0].Connectors[0].Overrides["lookup"] != console.AccessAsk {
		t.Fatalf("the review lists the sets %s", raw)
	}
	slots := dst.slots()
	if len(slots) != 3 {
		t.Fatalf("slots %+v", slots)
	}
	for name, mode := range map[string]console.ProfileMode{"alpha": console.ProfileCustom, "beta": console.ProfileOff, "gamma": console.ProfileFollow} {
		if c := slots[name].Connectors; len(c) != 1 || c[0].Mode != mode || c[0].Connector != dst.conn {
			t.Errorf("slot %s: %+v", name, slots[name])
		}
	}
	if o := slots["alpha"].Connectors[0].Overrides; len(o) != 1 || o["lookup"] != console.AccessAsk {
		t.Errorf("alpha's overrides %v", o)
	}
	if p := dst.profile("alpha"); p.Mode == console.ProfileCustom {
		t.Errorf("a key named like a set took it: %+v", p)
	}
	dst.mint("beta")
	if p := dst.profile("beta"); p.Mode != console.ProfileFollow {
		t.Errorf("a new key named like a set took it: %+v", p)
	}
	if got := dst.summary().Slots; len(got) != 3 {
		t.Errorf("the connector lists the sets %+v", got)
	}
	if len(dst.slots()) != 3 {
		t.Error("making keys consumed a set")
	}
}

func TestAPermissionSetIsClaimedWhenAnAppConnects(t *testing.T) {
	dst := importedSlots(t, slotSetup(t))
	dst.b.trustReads(dst.conn, "lookup", "file")
	agent := dst.consent("alpha")
	dst.hasExactly(agent, console.ProfileCustom, map[string]console.Access{"lookup": console.AccessAsk})
	if got := dst.effects(agent); got["lookup"] != console.AccessAsk || got["file"] != console.AccessOn {
		t.Fatalf("the engine answers %v", got)
	}
	if _, ok := dst.slots()["alpha"]; ok {
		t.Fatal("the set is still waiting once claimed")
	}
}

func TestAPermissionSetIsClaimedOnAKeyMadeByHand(t *testing.T) {
	dst := importedSlots(t, slotSetup(t))
	var tok console.AgentToken
	dst.b.ok("POST", "/api/agents", map[string]any{"name": "cc", "slot": "alpha"}, &tok)
	dst.hasExactly("cc", console.ProfileCustom, map[string]console.Access{"lookup": console.AccessAsk})
	if _, ok := dst.slots()["alpha"]; ok {
		t.Fatal("the set is still waiting once claimed")
	}
	if status, _ := dst.b.send("POST", "/api/agents", map[string]any{"name": "dd", "slot": "nobody"}, nil); status != http.StatusNotFound {
		t.Fatalf("an unknown set: %d", status)
	}
	if _, ok := dst.c.State.Agent("dd"); ok {
		t.Fatal("a key was made with an unknown set")
	}
}

func TestAPermissionSetIsAppliedToAKeyMadeAlready(t *testing.T) {
	dst := importedSlots(t, slotSetup(t), "delta")
	dst.b.ok("POST", "/api/slots/beta/claim", map[string]string{"key": "delta"}, nil)
	dst.hasExactly("delta", console.ProfileOff, nil)
	dst.b.ok("POST", "/api/slots/alpha/claim", map[string]string{"key": "delta"}, nil)
	dst.hasExactly("delta", console.ProfileCustom, map[string]console.Access{"lookup": console.AccessAsk})
	if s := dst.slots(); len(s) != 1 {
		t.Fatalf("slots left %+v", s)
	}
	if status, _ := dst.b.send("POST", "/api/slots/gamma/claim", map[string]string{"key": "nobody"}, nil); status != http.StatusNotFound {
		t.Fatalf("a claim for no key: %d", status)
	}
}

func TestAPermissionSetIsDiscarded(t *testing.T) {
	dst := importedSlots(t, slotSetup(t))
	dst.b.ok("DELETE", "/api/slots/gamma", nil, nil)
	if status, _ := dst.b.send("DELETE", "/api/slots/gamma", nil, nil); status != http.StatusNotFound {
		t.Fatalf("a second discard: %d", status)
	}
	if _, ok := dst.slots()["gamma"]; ok {
		t.Fatal("a discarded set is still waiting")
	}
}

func TestAPermissionSetNeverWidensBeyondTheConnector(t *testing.T) {
	file := bytes.Replace(slotSetup(t), []byte("lookup: ask"), []byte("lookup: \"on\""), 1)
	dst := importedSlots(t, file)
	dst.b.ok("PUT", "/api/connectors/"+dst.conn+"/functions/file", map[string]string{"access": "off"}, nil)
	dst.b.ok("PUT", "/api/connectors/"+dst.conn+"/functions/lookup", map[string]string{"access": "ask"}, nil)
	dst.b.ok("POST", "/api/agents", map[string]any{"name": "cc", "slot": "alpha"}, nil)
	if got := dst.effects("cc"); got["file"] != console.AccessOff || got["lookup"] != console.AccessAsk {
		t.Fatalf("a set widened the key: %v", got)
	}
}

func TestRemovingAConnectorDropsItFromEveryPermissionSet(t *testing.T) {
	dst := importedSlots(t, slotSetup(t))
	dst.b.ok("DELETE", "/api/connectors/"+dst.conn, nil, nil)
	if s := dst.c.State.Slots(); len(s) != 0 {
		t.Fatalf("slots left %+v", s)
	}
}

func TestWaitingProfilesBecomePermissionSets(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.json")
	old := `{"pending_profiles":{"c1":[{"agent":"alpha","mode":"custom","overrides":{"lookup":"off"}}],"c2":[{"agent":"alpha","mode":"off"},{"agent":"beta","mode":"off"}]}}`
	if err := os.WriteFile(path, []byte(old), 0o600); err != nil {
		t.Fatal(err)
	}
	state, err := console.OpenState(path, "owner")
	if err != nil {
		t.Fatal(err)
	}
	slots := state.Slots()
	if len(slots) != 2 || slots[0].Name != "alpha" || slots[1].Name != "beta" {
		t.Fatalf("slots %+v", slots)
	}
	if p := slots[0].Profiles; p["c1"].Mode != console.ProfileCustom || p["c1"].Overrides["lookup"] != console.AccessOff || p["c2"].Mode != console.ProfileOff {
		t.Fatalf("alpha %+v", p)
	}
	if p := slots[1].Profiles; len(p) != 1 || p["c2"].Mode != console.ProfileOff {
		t.Fatalf("beta %+v", p)
	}
}

func TestTheImportReviewDecidesEachPermissionSet(t *testing.T) {
	file := slotSetup(t)
	dst := profiled{stack: freshOwner(t, personRelays)}
	dst.mint("mine")
	opts := console.SetupOptions{Slots: map[string]console.SlotChoice{"alpha": {Action: console.SlotApply, Key: "mine"}, "beta": {Action: console.SlotDiscard}}}
	res := dst.importWith(file, opts)
	var conns struct{ Connectors []console.ConnectorSummary }
	dst.b.ok("GET", "/api/connectors", nil, &conns)
	dst.conn = conns.Connectors[0].ID
	dst.hasExactly("mine", console.ProfileCustom, map[string]console.Access{"lookup": console.AccessAsk})
	if len(res.Slots) != 1 || res.Slots[0] != "gamma" || len(dst.slots()) != 1 {
		t.Fatalf("result %+v, slots %+v", res, dst.slots())
	}

	t.Run("a set applied to no key made yet refuses the whole import", func(t *testing.T) {
		dst := freshOwner(t, personRelays)
		raw, _ := json.Marshal(console.SetupOptions{Slots: map[string]console.SlotChoice{"alpha": {Action: console.SlotApply, Key: "nobody"}}})
		if status, out := dst.uploadForm("/api/backup/import", file, map[string]string{"options": string(raw)}); status != http.StatusNotFound {
			t.Fatalf("import: %d %s", status, out)
		}
		if len(dst.choices()) != 0 {
			t.Fatal("a refused import installed something")
		}
	})

	t.Run("a set that fails to apply is reported once and kept", func(t *testing.T) {
		dst := profiled{stack: freshOwner(t, personRelays)}
		dst.mint("mine")
		dst.c.Engine.HTTP = &http.Client{Transport: refuseKeyOverrides{next: http.DefaultTransport}}
		res := dst.importWith(file, console.SetupOptions{Slots: map[string]console.SlotChoice{"alpha": {Action: console.SlotApply, Key: "mine"}}})
		dst.c.Engine.HTTP = &http.Client{}
		var conns struct{ Connectors []console.ConnectorSummary }
		dst.b.ok("GET", "/api/connectors", nil, &conns)
		dst.conn = conns.Connectors[0].ID
		if _, failed := res.Failed["permission set alpha"]; !failed || len(res.Failed) != 1 || !slices.Contains(res.Slots, "alpha") {
			t.Fatalf("result %+v", res)
		}
		if p := dst.profile("mine"); p.Mode != console.ProfileOff {
			t.Fatalf("mine after a failed apply: %+v", p)
		}
	})
}

type refuseKeyOverrides struct{ next http.RoundTripper }

func (r refuseKeyOverrides) RoundTrip(req *http.Request) (*http.Response, error) {
	if req.Method == http.MethodPost && req.URL.Path == "/v1/grants" && req.Body != nil {
		raw, _ := io.ReadAll(req.Body)
		req.Body = io.NopCloser(bytes.NewReader(raw))
		if bytes.Contains(raw, []byte(`"scope_kind":"agent"`)) && !bytes.Contains(raw, []byte(`"mode":"allow"`)) {
			return &http.Response{StatusCode: http.StatusInternalServerError, Body: io.NopCloser(strings.NewReader(`{"code":"internal","detail":"refused"}`)), Header: http.Header{}, Request: req}, nil
		}
	}
	return r.next.RoundTrip(req)
}
