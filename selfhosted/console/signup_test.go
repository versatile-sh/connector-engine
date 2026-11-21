package console_test

import (
	"net/http"
	"strings"
	"testing"

	"github.com/versatile-sh/connector-engine/selfhosted/console"
)

const signupPassword = "a brand new password"

func (r *accountRig) signupOn(role string) {
	r.t.Helper()
	owner, _ := r.signIn("me", "correct horse battery")
	owner.ok("PUT", "/api/people/signup", map[string]any{"enabled": true, "role": role}, nil)
}

func (r *accountRig) signup(b *browser, username, password string) (int, string) {
	status, raw := b.send("POST", "/api/signup", map[string]string{"username": username, "password": password}, nil)
	return status, string(raw)
}

func TestSignupIsClosedByDefaultAndTheSessionSaysSo(t *testing.T) {
	r := newAccountRig(t)
	b := r.browser()
	if status, raw := r.signup(b, "ann", signupPassword); status != http.StatusForbidden || !strings.Contains(raw, "signup_closed") {
		t.Fatalf("closed: %d %s", status, raw)
	}
	var s struct {
		SignupOpen bool `json:"signup_open"`
	}
	b.ok("GET", "/api/session", nil, &s)
	if s.SignupOpen {
		t.Fatal("the session says sign-up is open")
	}
}

func TestSignupMakesAMemberSignedInAndTheSessionSaysItIsOpen(t *testing.T) {
	r := newAccountRig(t)
	r.signupOn("member")
	b := r.browser()
	var out struct{ CSRF string }
	b.ok("POST", "/api/signup", map[string]string{"username": " Ann ", "password": signupPassword}, &out)
	b.csrf = out.CSRF
	var s struct {
		SignupOpen bool `json:"signup_open"`
		User       struct{ Username, Role string }
	}
	b.ok("GET", "/api/session", nil, &s)
	if !s.SignupOpen || s.User.Username != "Ann" || s.User.Role != "member" {
		t.Fatalf("session %+v", s)
	}
	if _, status := r.signIn("ann", signupPassword); status != http.StatusOK {
		t.Fatalf("sign-in after sign-up: %d", status)
	}
}

func TestSignupRoleIsTheOwnersChoiceAndNeverTheCallers(t *testing.T) {
	r := newAccountRig(t)
	r.signupOn("member")
	owner, _ := r.signIn("me", "correct horse battery")
	b := r.browser()
	status, _ := b.send("POST", "/api/signup", map[string]string{"username": "eve", "password": signupPassword, "role": "owner"}, nil)
	if status != http.StatusCreated {
		t.Fatalf("sign-up: %d", status)
	}
	_, data := r.people(owner)
	for _, p := range data.People {
		if p.Username == "eve" && p.Role != console.RoleMember {
			t.Fatalf("eve is %s", p.Role)
		}
	}
	if status, _ := owner.send("PUT", "/api/people/signup", map[string]any{"enabled": true, "role": "owner"}, nil); status != http.StatusBadRequest {
		t.Fatalf("owner as the sign-up role: %d", status)
	}
	owner.ok("PUT", "/api/people/signup", map[string]any{"enabled": true, "role": "admin"}, nil)
	r.signup(r.browser(), "dan", signupPassword)
	_, data = r.people(owner)
	for _, p := range data.People {
		if p.Username == "dan" && p.Role != console.RoleAdmin {
			t.Fatalf("dan is %s", p.Role)
		}
	}
}

func TestSignupRefusesATakenUsernameInAnyCaseAndAShortPassword(t *testing.T) {
	r := newAccountRig(t)
	r.signupOn("member")
	for _, name := range []string{"me", "ME", "Me"} {
		if status, _ := r.signup(r.browser(), name, signupPassword); status != http.StatusConflict {
			t.Errorf("%q: %d", name, status)
		}
	}
	if status, _ := r.signup(r.browser(), "ann", "short"); status != http.StatusBadRequest {
		t.Errorf("short password: %d", status)
	}
}

func TestSignupIsRateLimitedPerAddress(t *testing.T) {
	r := newAccountRig(t)
	r.signupOn("member")
	var last int
	for i := range 7 {
		last, _ = r.signup(r.browser(), "user"+string(rune('a'+i)), signupPassword)
	}
	if last != http.StatusTooManyRequests {
		t.Fatalf("the seventh sign-up: %d", last)
	}
}

func TestSignupHonoursTheOriginGuard(t *testing.T) {
	r := newAccountRig(t)
	r.signupOn("member")
	status, _ := r.browser().send("POST", "/api/signup", map[string]string{"username": "ann", "password": signupPassword}, map[string]string{"Origin": "https://evil.example"})
	if status != http.StatusForbidden {
		t.Fatalf("foreign origin: %d", status)
	}
}

func TestOnlyAnOwnerChangesSignupAndEveryChangeIsAudited(t *testing.T) {
	r := newAccountRig(t)
	admin, _ := r.join("admin", "ann")
	member, _ := r.join("member", "bob")
	for who, b := range map[string]*browser{"admin": admin, "member": member} {
		if status, _ := b.send("PUT", "/api/people/signup", map[string]any{"enabled": true, "role": "member"}, nil); status != http.StatusForbidden {
			t.Errorf("%s: %d", who, status)
		}
	}
	owner, _ := r.signIn("me", "correct horse battery")
	owner.ok("PUT", "/api/people/signup", map[string]any{"enabled": true, "role": "member"}, nil)
	owner.ok("PUT", "/api/people/signup", map[string]any{"enabled": false, "role": "member"}, nil)
	_, data := r.people(owner)
	changes := 0
	for _, a := range data.Audit {
		if a.Action == "signup.changed" {
			changes++
		}
	}
	if changes != 2 {
		t.Fatalf("audited %d changes", changes)
	}
	var list struct{ Signup console.SignupSettings }
	owner.ok("GET", "/api/people", nil, &list)
	if list.Signup.Enabled || list.Signup.Role != console.RoleMember {
		t.Fatalf("settings %+v", list.Signup)
	}
}

func TestSignupNeverMakesTheFirstOwner(t *testing.T) {
	state, err := console.OpenState(t.TempDir()+"/state.json", "owner")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := state.SignUp("ann", signupPassword); err != console.ErrSignupClosed {
		t.Fatalf("got %v", err)
	}
}
