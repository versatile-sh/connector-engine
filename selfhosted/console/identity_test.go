package console

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/versatile-sh/connector-engine/selfhosted/surface"
)

type recordingEngine struct {
	mu     sync.Mutex
	mints  []map[string]any
	writes []string
	reads  []string
}

func (e *recordingEngine) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	e.mu.Lock()
	defer e.mu.Unlock()
	if r.URL.Path == "/local/token" {
		var claims map[string]any
		_ = json.NewDecoder(r.Body).Decode(&claims)
		e.mints = append(e.mints, claims)
		_ = json.NewEncoder(w).Encode(map[string]string{"token": "tok:" + claims["sub"].(string)})
		return
	}
	line := r.Method + " " + r.URL.Path + " as " + strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer tok:")
	if r.Method == http.MethodGet {
		e.reads = append(e.reads, line)
	} else {
		e.writes = append(e.writes, line)
	}
	switch {
	case strings.HasPrefix(r.URL.Path, "/v1/approvals/") && r.Method == http.MethodGet:
		_, _ = w.Write([]byte(`{"approval":{"id":"a1","agent":"bobkey","state":"pending","mode":"ask"}}`))
	default:
		_, _ = w.Write([]byte(`{}`))
	}
}

func (e *recordingEngine) seen() (mints []map[string]any, writes, reads []string) {
	e.mu.Lock()
	defer e.mu.Unlock()
	return slices.Clone(e.mints), slices.Clone(e.writes), slices.Clone(e.reads)
}

func recordedConsole(t *testing.T) (*Console, *recordingEngine) {
	t.Helper()
	rec := &recordingEngine{}
	srv := httptest.NewServer(rec)
	t.Cleanup(srv.Close)
	c := userConsole(t, filepath.Join(t.TempDir(), "state.json"))
	c.Engine = &Engine{PublicURL: srv.URL, OperatorURL: srv.URL, Operator: "op", Tenant: "local", Workspace: "default", ServicePerson: "console"}
	addUser(t, c.State, "owner", "me", RoleOwner)
	addUser(t, c.State, "p-bob", "bob", RoleMember)
	if err := c.State.PutAgent(Agent{Name: "bobkey", Owner: "p-bob", ExpiresAt: time.Now().Add(time.Hour)}); err != nil {
		t.Fatal(err)
	}
	return c, rec
}

func TestAnEngineWriteWithoutAnActorIsRefusedAndSendsNothing(t *testing.T) {
	c, rec := recordedConsole(t)
	if err := c.Engine.Call(context.Background(), http.MethodPut, "/v1/grants", map[string]any{}, nil); !errors.Is(err, ErrNoActor) {
		t.Fatalf("a write without an actor: %v", err)
	}
	if _, err := c.BindAgent(context.Background(), "laptop", time.Hour, ""); !errors.Is(err, ErrNoActor) {
		t.Fatalf("a key made without an actor: %v", err)
	}
	if mints, writes, reads := rec.seen(); len(mints)+len(writes)+len(reads) != 0 {
		t.Fatalf("the engine saw %v %v %v", mints, writes, reads)
	}
}

func TestEachEngineWriteRunsAsTheActingUserAndEachReadAsTheServicePerson(t *testing.T) {
	c, rec := recordedConsole(t)
	if err := c.State.SetCredentialMode("c1", CredentialPerPerson); err != nil {
		t.Fatal(err)
	}
	cookie := sessionOf(t, c.State, "p-bob")
	var csrf string
	for _, s := range c.State.data.Sessions {
		csrf = s.CSRF
	}
	req := httptest.NewRequest(http.MethodPost, "/api/connectors/c1/headers", strings.NewReader(`{"headers":{"X-Key":"v"}}`))
	req.AddCookie(cookie)
	req.SetPathValue("id", "c1")
	req.Header.Set(csrfHeader, csrf)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	c.authed(c.setHeaders)(w, req)
	if w.Code != http.StatusNoContent {
		t.Fatalf("headers answered %d %s", w.Code, w.Body)
	}
	if _, err := c.Approvals(context.Background()); err != nil {
		t.Fatal(err)
	}
	if _, err := (desk{c: c}).Decide(WithActor(context.Background(), "p-bob"), "a1", surface.ChoiceOnce, "telegram", "telegram:me"); err != nil {
		t.Fatal(err)
	}
	if _, err := c.agentToken(context.Background(), "bobkey", time.Hour); err != nil {
		t.Fatal(err)
	}
	mints, writes, reads := rec.seen()
	if want := []string{"POST /v1/credentials as p-bob", "POST /v1/approvals/a1:decide as p-bob"}; !slices.Equal(writes, want) {
		t.Errorf("writes %v, want %v", writes, want)
	}
	for _, r := range reads {
		if !strings.HasSuffix(r, " as console") {
			t.Errorf("read %s", r)
		}
	}
	last := mints[len(mints)-1]
	if last["sub"] != "bobkey" || last["on_behalf_of"] != "p-bob" || last["agent_owner"] != "p-bob" {
		t.Errorf("the agent token claims %v", last)
	}
	persons := 0
	for _, m := range mints {
		if m["agent"] == nil {
			persons++
		}
	}
	if persons != 2 {
		t.Errorf("person tokens minted %d times for two persons: %v", persons, mints)
	}
}

func TestAKeyKeepsItsOwnerAndAnOwnerlessKeyBelongsToTheLegacyPerson(t *testing.T) {
	c, _ := recordedConsole(t)
	if err := c.State.PutAgent(Agent{Name: "bobkey", Owner: "owner"}); !errors.Is(err, ErrNameTaken) {
		t.Fatalf("another owner for a key: %v", err)
	}
	if err := c.State.PutAgent(Agent{Name: "bobkey", ExpiresAt: time.Now().Add(2 * time.Hour)}); err != nil {
		t.Fatal(err)
	}
	if a, _ := c.State.Agent("bobkey"); a.Owner != "p-bob" {
		t.Fatalf("a renewal changed the owner to %q", a.Owner)
	}
	if err := c.State.PutAgent(Agent{Name: "nobody"}); err == nil {
		t.Fatal("a key without an owner was recorded")
	}
	d := stateFile{Agents: map[string]Agent{"old": {Name: "old"}}}
	migrateUsers(&d, "owner")
	if d.Agents["old"].Owner != "owner" {
		t.Fatalf("a key from before multi-user belongs to %q", d.Agents["old"].Owner)
	}
}

func TestAProviderSignInCompletesAsThePersonWhoBeganIt(t *testing.T) {
	c, rec := recordedConsole(t)
	c.connects.add("s1", "https://issuer.test", "p-bob")
	w := httptest.NewRecorder()
	c.oauthCallback(w, httptest.NewRequest(http.MethodGet, "/oauth/callback?state=s1&code=c1", nil))
	if loc := w.Header().Get("Location"); loc != "/connectors?oauth=connected" {
		t.Fatalf("the callback went to %q", loc)
	}
	if _, writes, _ := rec.seen(); !slices.Equal(writes, []string{"POST /v1/credentials:complete as p-bob"}) {
		t.Fatalf("writes %v", writes)
	}
}

func TestAProviderSignInBegunByAnotherUserIsRefusedInThisUsersBrowser(t *testing.T) {
	c, rec := recordedConsole(t)
	c.connects.add("s1", "https://issuer.test", "p-bob")
	req := httptest.NewRequest(http.MethodGet, "/oauth/callback?state=s1&code=c1", nil)
	req.AddCookie(sessionOf(t, c.State, "owner"))
	w := httptest.NewRecorder()
	c.oauthCallback(w, req)
	if loc := w.Header().Get("Location"); loc != "/connectors?oauth=wrong_user" {
		t.Fatalf("the callback went to %q", loc)
	}
	if _, writes, _ := rec.seen(); len(writes) != 0 {
		t.Fatalf("writes %v", writes)
	}
}
