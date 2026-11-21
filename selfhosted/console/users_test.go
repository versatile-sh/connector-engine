package console

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

const userPassword = "correct horse battery"

func userConsole(t *testing.T, path string) *Console {
	t.Helper()
	state, err := OpenState(path, "owner")
	if err != nil {
		t.Fatal(err)
	}
	return &Console{State: state, Sessions: NewSessions(state)}
}

func addUser(t *testing.T, s *State, person, username string, role Role) {
	t.Helper()
	u, err := newCredentials(username, userPassword)
	if err != nil {
		t.Fatal(err)
	}
	u.Person, u.Role = person, role
	if err := s.update(func(d *stateFile) error { put(&d.Users, person, u); return nil }); err != nil {
		t.Fatal(err)
	}
}

func sessionOf(t *testing.T, s *State, person string) *http.Cookie {
	t.Helper()
	rec := httptest.NewRecorder()
	if _, err := NewSessions(s).Start(rec, person); err != nil {
		t.Fatal(err)
	}
	return rec.Result().Cookies()[0]
}

func accountWith(c *Console, cookie *http.Cookie) (int, string) {
	req := httptest.NewRequest(http.MethodGet, "/api/account", nil)
	req.AddCookie(cookie)
	rec := httptest.NewRecorder()
	c.authed(c.account)(rec, req)
	return rec.Code, rec.Body.String()
}

func TestAStateFileFromBeforeMultiUserBecomesItsOwnerAndKeepsItsSession(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.json")
	creds, err := newCredentials("me", userPassword)
	if err != nil {
		t.Fatal(err)
	}
	id := randomID()
	legacy := map[string]any{
		"owner":    map[string]any{"username": creds.Username, "salt": creds.Salt, "hash": creds.Hash, "iterations": creds.Iterations},
		"sessions": map[string]any{sessionKey(id): map[string]any{"csrf": "c1", "expires": time.Now().Add(time.Hour)}},
	}
	raw, _ := json.Marshal(legacy)
	if err := os.WriteFile(path, raw, 0o600); err != nil {
		t.Fatal(err)
	}
	state, err := OpenState(path, "alice-person")
	if err != nil {
		t.Fatal(err)
	}
	u, ok := state.User("alice-person")
	if !ok || u.Role != RoleOwner || u.Username != "me" || state.data.LegacyPerson != "alice-person" {
		t.Fatalf("migrated to %+v %v, legacy person %q", u, ok, state.data.LegacyPerson)
	}
	c := &Console{State: state, Sessions: NewSessions(state)}
	if code, body := accountWith(c, &http.Cookie{Name: sessionCookie, Value: id}); code != http.StatusOK || !strings.Contains(body, `"username":"me"`) {
		t.Fatalf("the legacy session: %d %s", code, body)
	}
	if _, ok, err := state.CheckLogin("me", userPassword); !ok || err != nil {
		t.Fatalf("the legacy password: %v %v", ok, err)
	}
	if err := state.DismissOnboarding(); err != nil {
		t.Fatal(err)
	}
	written, _ := os.ReadFile(path)
	var back map[string]json.RawMessage
	_ = json.Unmarshal(written, &back)
	if _, has := back["owner"]; has || back["users"] == nil {
		t.Fatalf("the next write kept the legacy shape: %s", written)
	}
}

func TestALegacyStatesChannelsAndDevicesBelongToTheFirstOwner(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.json")
	creds, err := newCredentials("me", userPassword)
	if err != nil {
		t.Fatal(err)
	}
	legacy := map[string]any{
		"owner": map[string]any{"username": creds.Username, "salt": creds.Salt, "hash": creds.Hash, "iterations": creds.Iterations},
		"notifications": map[string]any{
			"channels": map[string]any{"ch1": map[string]any{"id": "ch1", "kind": "ntfy", "label": "phone"}},
			"devices":  map[string]any{"dev1": map[string]any{"id": "dev1", "name": "laptop", "key_hash": "abc"}},
		},
	}
	raw, _ := json.Marshal(legacy)
	if err := os.WriteFile(path, raw, 0o600); err != nil {
		t.Fatal(err)
	}
	state, err := OpenState(path, "alice-person")
	if err != nil {
		t.Fatal(err)
	}
	if chans := state.Channels(); len(chans) != 1 || chans[0].Owner != "alice-person" {
		t.Fatalf("the legacy channels: %+v", chans)
	}
	if dev, ok := state.Device("dev1"); !ok || dev.Owner != "alice-person" {
		t.Fatalf("the legacy device: %+v %v", dev, ok)
	}
}

func TestTheLastOwnerCanBeNeitherDemotedNorRemoved(t *testing.T) {
	c := userConsole(t, filepath.Join(t.TempDir(), "state.json"))
	if _, err := c.State.CreateFirstOwner("me", userPassword); err != nil {
		t.Fatal(err)
	}
	for name, err := range map[string]error{"demoted": c.State.SetRole("owner", RoleAdmin), "removed": c.State.RemoveUser("owner")} {
		if !errors.Is(err, ErrLastOwner) {
			t.Errorf("the last owner %s: %v", name, err)
		}
	}
	addUser(t, c.State, "p-2", "bob", RoleOwner)
	if err := c.State.SetRole("owner", RoleMember); err != nil {
		t.Fatalf("demoting one of two owners: %v", err)
	}
	if err := c.State.RemoveUser("p-2"); !errors.Is(err, ErrLastOwner) {
		t.Fatalf("removing the remaining owner: %v", err)
	}
	if err := c.State.SetRole("p-2", Role("root")); err == nil {
		t.Fatal("an unknown role was set")
	}
}

func TestOneUsersPasswordChangeKeepsTheOtherUsersSessions(t *testing.T) {
	c := userConsole(t, filepath.Join(t.TempDir(), "state.json"))
	if _, err := c.State.CreateFirstOwner("me", userPassword); err != nil {
		t.Fatal(err)
	}
	addUser(t, c.State, "p-2", "bob", RoleMember)
	mine, myOther, bobs := sessionOf(t, c.State, "owner"), sessionOf(t, c.State, "owner"), sessionOf(t, c.State, "p-2")
	if err := c.State.SetPassword("owner", "another long password", sessionKey(mine.Value)); err != nil {
		t.Fatal(err)
	}
	for name, want := range map[*http.Cookie]int{mine: http.StatusOK, myOther: http.StatusUnauthorized, bobs: http.StatusOK} {
		if code, _ := accountWith(c, name); code != want {
			t.Errorf("session %s: %d, want %d", name.Value[:6], code, want)
		}
	}
	if _, ok, _ := c.State.CheckLogin("bob", userPassword); !ok {
		t.Fatal("bob's password changed")
	}
}

func TestASessionOfARemovedUserIsRefused(t *testing.T) {
	c := userConsole(t, filepath.Join(t.TempDir(), "state.json"))
	if _, err := c.State.CreateFirstOwner("me", userPassword); err != nil {
		t.Fatal(err)
	}
	addUser(t, c.State, "p-2", "bob", RoleMember)
	bobs := sessionOf(t, c.State, "p-2")
	if code, _ := accountWith(c, bobs); code != http.StatusOK {
		t.Fatalf("before removal: %d", code)
	}
	if err := c.State.RemoveUser("p-2"); err != nil {
		t.Fatal(err)
	}
	if code, _ := accountWith(c, bobs); code != http.StatusUnauthorized {
		t.Fatalf("after removal: %d", code)
	}
	orphan := sessionOf(t, c.State, "p-gone")
	if code, _ := accountWith(c, orphan); code != http.StatusUnauthorized {
		t.Fatalf("a session of no user: %d", code)
	}
}

func TestUsernamesAreUniqueAndFoundRegardlessOfCase(t *testing.T) {
	c := userConsole(t, filepath.Join(t.TempDir(), "state.json"))
	if _, err := c.State.CreateFirstOwner("Me", userPassword); err != nil {
		t.Fatal(err)
	}
	addUser(t, c.State, "p-2", "bob", RoleMember)
	if err := c.State.Rename("p-2", "ME"); err == nil {
		t.Fatal("bob took the owner's username in another case")
	}
	if err := c.State.Rename("owner", "me"); err != nil {
		t.Fatalf("renaming oneself in another case: %v", err)
	}
	if u, ok, err := c.State.CheckLogin("ME", userPassword); !ok || err != nil || u.Person != "owner" {
		t.Fatalf("a login in another case: %+v %v %v", u, ok, err)
	}
}

func TestRolesAreAClosedOrderedSet(t *testing.T) {
	for _, s := range []string{"owner", "admin", "member"} {
		if r, err := ParseRole(s); err != nil || string(r) != s {
			t.Errorf("%s: %v", s, err)
		}
	}
	for _, s := range []string{"", "Owner", "root"} {
		if _, err := ParseRole(s); err == nil {
			t.Errorf("%q was accepted", s)
		}
	}
	if !RoleOwner.AtLeast(RoleAdmin) || !RoleAdmin.AtLeast(RoleMember) || RoleMember.AtLeast(RoleAdmin) || Role("root").AtLeast(RoleMember) {
		t.Fatal("the role order is wrong")
	}
}

func TestTheHostResetSetsTheNamedUsersPasswordOrTheOnlyOne(t *testing.T) {
	c := userConsole(t, filepath.Join(t.TempDir(), "state.json"))
	if _, err := c.State.CreateFirstOwner("me", userPassword); err != nil {
		t.Fatal(err)
	}
	reset := func(body string) int {
		rec := httptest.NewRecorder()
		c.HostHandler().ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/password", strings.NewReader(body)))
		return rec.Code
	}
	if code := reset("a reset password\n"); code != http.StatusNoContent {
		t.Fatalf("one user, no name: %d", code)
	}
	if _, ok, _ := c.State.CheckLogin("me", "a reset password"); !ok {
		t.Fatal("the only user's password was not reset")
	}
	addUser(t, c.State, "p-2", "bob", RoleMember)
	bobs := sessionOf(t, c.State, "p-2")
	mine := sessionOf(t, c.State, "owner")
	if code := reset("\nyet another password"); code != http.StatusBadRequest {
		t.Fatalf("two users, no name: %d", code)
	}
	if code := reset("nobody\nyet another password"); code != http.StatusNotFound {
		t.Fatalf("an unknown name: %d", code)
	}
	if code := reset("BOB\nyet another password"); code != http.StatusNoContent {
		t.Fatalf("a named user: %d", code)
	}
	if _, ok, _ := c.State.CheckLogin("bob", "yet another password"); !ok {
		t.Fatal("bob's password was not reset")
	}
	if code, _ := accountWith(c, bobs); code != http.StatusUnauthorized {
		t.Fatalf("bob's session after the reset: %d", code)
	}
	if code, _ := accountWith(c, mine); code != http.StatusOK {
		t.Fatalf("the owner's session after bob's reset: %d", code)
	}
}
