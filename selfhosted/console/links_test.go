package console

import (
	"encoding/hex"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/versatile-sh/connector-engine/secrets/memory"
	"github.com/versatile-sh/connector-engine/selfhosted/console/sealed"
)

const legacyKey = "00112233445566778899aabbccddeeff00112233445566778899aabbccddeeff"

func linkConsole(t *testing.T, path string, box *sealed.Box) *Console {
	t.Helper()
	state, err := OpenState(path, "owner")
	if err != nil {
		t.Fatal(err)
	}
	return &Console{State: state, Sessions: NewSessions(state), Secrets: box}
}

func TestALockedConsoleAnswersLinksWithTheUnlockHint(t *testing.T) {
	c := linkConsole(t, filepath.Join(t.TempDir(), "state.json"), &sealed.Box{})
	if _, err := c.LinkToken("a1", "d1", time.Now().Add(time.Hour)); !errors.Is(err, sealed.ErrLocked) {
		t.Fatalf("a link minted while locked: %v", err)
	}
	for _, req := range []*http.Request{
		httptest.NewRequest(http.MethodGet, "/api/approvals/a1?t=x.y", nil),
		httptest.NewRequest(http.MethodPost, "/api/approvals/a1/decide", strings.NewReader(`{"choice":"once","token":"x.y"}`)),
	} {
		req.SetPathValue("id", "a1")
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		if req.Method == http.MethodGet {
			c.approvalPage(rec, req)
		} else {
			c.decideChoice(rec, req)
		}
		if rec.Code != http.StatusServiceUnavailable || !strings.Contains(rec.Body.String(), "make unlock") {
			t.Errorf("%s while locked: %d %s", req.Method, rec.Code, rec.Body)
		}
	}
}

func TestAPlaintextLinkKeyIsSealedOnTheFirstUnlockAndKeepsItsLinks(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.json")
	if err := os.WriteFile(path, []byte(`{"notifications":{"link_key":"`+legacyKey+`"}}`), 0o600); err != nil {
		t.Fatal(err)
	}
	box, keys := &sealed.Box{}, memory.New()
	c := linkConsole(t, path, box)
	if _, err := c.linkKey(); !errors.Is(err, sealed.ErrLocked) {
		t.Fatalf("the plaintext key was used while locked: %v", err)
	}
	box.Unlock(keys)
	key, err := c.linkKey()
	if err != nil || hex.EncodeToString(key) != legacyKey {
		t.Fatalf("the key changed at migration, so outstanding links would break: %x %v", key, err)
	}
	raw, _ := os.ReadFile(path)
	if strings.Contains(string(raw), legacyKey) || !strings.Contains(string(raw), "sealed_link_key") {
		t.Fatalf("the state file after the first unlock: %s", raw)
	}
	token, err := c.LinkToken("a1", "d1", time.Now().Add(time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	restarted := &sealed.Box{}
	restarted.Unlock(keys)
	again := linkConsole(t, path, restarted)
	if _, fields, err := again.open(linkKind, token); err != nil || len(fields) != 2 || fields[0] != "a1" {
		t.Fatalf("a link after a restart: %v %v", fields, err)
	}
}

func TestANewLinkKeyIsSealedFromTheStart(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.json")
	box := &sealed.Box{}
	box.Unlock(memory.New())
	c := linkConsole(t, path, box)
	token, err := c.LinkToken("a1", "d1", time.Now().Add(time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := c.open(linkKind, token); err != nil {
		t.Fatalf("a fresh link: %v", err)
	}
	raw, _ := os.ReadFile(path)
	if strings.Contains(string(raw), `"link_key"`) || !strings.Contains(string(raw), "sealed_link_key") {
		t.Fatalf("the state file holds a plaintext key: %s", raw)
	}
}
