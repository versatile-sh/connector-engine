package console_test

import (
	"encoding/json"
	"net/http"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/versatile-sh/connector-engine/selfhosted/console"
)

type linkOut struct {
	ID        string `json:"id"`
	Link      string `json:"link"`
	ExpiresAt string `json:"expires_at"`
}

func (l linkOut) token(t *testing.T) string {
	t.Helper()
	_, frag, ok := strings.Cut(l.Link, "/invite#")
	if !ok || frag == "" {
		t.Fatalf("the link %q has no token fragment", l.Link)
	}
	tok, err := url.PathUnescape(frag)
	if err != nil {
		t.Fatal(err)
	}
	return tok
}

func (r *accountRig) people(b *browser) (int, struct {
	People  []console.PersonRow  `json:"people"`
	Invites []console.LinkRow    `json:"invites"`
	Audit   []console.AuditEntry `json:"audit"`
}) {
	r.t.Helper()
	var out struct {
		People  []console.PersonRow  `json:"people"`
		Invites []console.LinkRow    `json:"invites"`
		Audit   []console.AuditEntry `json:"audit"`
	}
	status, raw := b.send("GET", "/api/people", nil, nil)
	if status == http.StatusOK {
		if err := json.Unmarshal(raw, &out); err != nil {
			r.t.Fatal(err)
		}
	}
	return status, out
}

func (r *accountRig) join(role string, username string) (*browser, string) {
	r.t.Helper()
	owner, _ := r.signIn("me", "correct horse battery")
	var l linkOut
	owner.ok("POST", "/api/invites", map[string]string{"role": role}, &l)
	other := r.browser()
	var out struct{ CSRF string }
	other.ok("POST", "/api/invite/accept", map[string]string{"token": l.token(r.t), "username": username, "password": "a brand new password"}, &out)
	other.csrf = out.CSRF
	var s struct {
		User struct{ Person string }
	}
	other.ok("GET", "/api/session", nil, &s)
	return other, s.User.Person
}

func TestAnOwnerInvitesAPersonWhoOpensTheLinkOnTheConsolesOwnAddressAndJoins(t *testing.T) {
	r := newAccountRig(t)
	r.c.PublicURL = r.srv.URL
	owner, _ := r.signIn("me", "correct horse battery")

	var l linkOut
	owner.ok("POST", "/api/invites", map[string]string{"role": "admin"}, &l)
	if !strings.HasPrefix(l.Link, r.srv.URL+"/invite#") {
		t.Fatalf("the link %q is not on the console's address", l.Link)
	}
	if _, err := time.Parse(time.RFC3339, l.ExpiresAt); err != nil {
		t.Fatalf("expires_at %q", l.ExpiresAt)
	}
	stranger := r.browser()

	t.Run("inspecting says who invited and for which role without using the link", func(t *testing.T) {
		for range 2 {
			var facts console.LinkFacts
			stranger.ok("POST", "/api/invite/inspect", map[string]string{"token": l.token(t)}, &facts)
			if facts.Purpose != console.PurposeJoin || facts.Role != console.RoleAdmin || facts.InvitedBy != "me" || facts.Username != "" {
				t.Fatalf("facts %+v", facts)
			}
		}
	})

	t.Run("a wrong token is gone and a taken username is refused without using the link", func(t *testing.T) {
		if status, raw := stranger.send("POST", "/api/invite/inspect", map[string]string{"token": "nope"}, nil); status != http.StatusGone || !strings.Contains(string(raw), "link_gone") {
			t.Fatalf("a wrong token: %d %s", status, raw)
		}
		if status, _ := stranger.send("POST", "/api/invite/accept", map[string]string{"token": l.token(t), "username": "ME", "password": "a brand new password"}, nil); status != http.StatusConflict {
			t.Fatalf("a taken username: %d", status)
		}
		if status, _ := stranger.send("POST", "/api/invite/accept", map[string]string{"token": l.token(t), "username": "ann", "password": "short"}, nil); status != http.StatusBadRequest {
			t.Fatalf("a short password: %d", status)
		}
	})

	var csrf struct{ CSRF string }
	t.Run("accepting makes the user, signs them in and spends the link", func(t *testing.T) {
		stranger.ok("POST", "/api/invite/accept", map[string]string{"token": l.token(t), "username": " ann ", "password": "a brand new password"}, &csrf)
		stranger.csrf = csrf.CSRF
		var s struct {
			User struct{ Username, Role, Person string }
			Can  []string
		}
		stranger.ok("GET", "/api/session", nil, &s)
		if s.User.Username != "ann" || s.User.Role != "admin" || !strings.HasPrefix(s.User.Person, "p-") || len(s.User.Person) != 18 {
			t.Fatalf("the new user %+v", s.User)
		}
		if status, _ := r.browser().send("POST", "/api/invite/accept", map[string]string{"token": l.token(t), "username": "bob", "password": "a brand new password"}, nil); status != http.StatusGone {
			t.Fatalf("a second use: %d", status)
		}
		if _, status := r.signIn("ann", "a brand new password"); status != http.StatusOK {
			t.Fatalf("the new user cannot sign in: %d", status)
		}
	})

	t.Run("the people list shows both, the audit names the change and nothing secret", func(t *testing.T) {
		status, data := r.people(owner)
		if status != http.StatusOK || len(data.People) != 2 || len(data.Invites) != 0 {
			t.Fatalf("people %d %+v", status, data)
		}
		if p := data.People[0]; p.Username != "me" || !p.You || p.Role != console.RoleOwner {
			t.Fatalf("the first row %+v", p)
		}
		if p := data.People[1]; p.Username != "ann" || p.You || !p.MayReset || p.CreatedAt.IsZero() {
			t.Fatalf("the second row %+v", p)
		}
		actions := []string{}
		for _, a := range data.Audit {
			actions = append(actions, a.Action)
		}
		if strings.Join(actions, ",") != "user.created,invite.created,user.created" {
			t.Fatalf("audit %v", actions)
		}
		_, raw := owner.send("GET", "/api/people", nil, nil)
		for _, secret := range []string{l.token(t), "hash", "salt", "password"} {
			if strings.Contains(string(raw), secret) {
				t.Errorf("the people list carries %q", secret)
			}
		}
		state, _ := os.ReadFile(r.path)
		if strings.Contains(string(state), l.token(t)) {
			t.Error("the state file keeps the link token")
		}
	})
}

func TestOnlyAnOwnerManagesPeopleAndAnAdminSeesThemOnTheirList(t *testing.T) {
	r := newAccountRig(t)
	admin, _ := r.join("admin", "ann")
	member, memberP := r.join("member", "bob")
	owner, _ := r.signIn("me", "correct horse battery")

	if status, _ := r.people(admin); status != http.StatusOK {
		t.Errorf("an Admin reading people: %d", status)
	}
	if status, _ := r.people(member); status != http.StatusForbidden {
		t.Errorf("a Member reading people: %d", status)
	}
	for who, b := range map[string]*browser{"admin": admin, "member": member} {
		for _, c := range []struct{ method, path string }{
			{"POST", "/api/invites"}, {"PUT", "/api/people/" + memberP + "/role"}, {"DELETE", "/api/people/" + memberP},
		} {
			if status, _ := b.send(c.method, c.path, map[string]string{"role": "owner"}, nil); status != http.StatusForbidden {
				t.Errorf("%s %s %s answered %d", who, c.method, c.path, status)
			}
		}
	}
	if status, _ := member.send("POST", "/api/people/"+memberP+"/reset", nil, nil); status != http.StatusForbidden {
		t.Errorf("a Member's reset link: %d", status)
	}
	if status, _ := owner.send("POST", "/api/invites", map[string]string{"role": "root"}, nil); status != http.StatusBadRequest {
		t.Errorf("an unknown role: %d", status)
	}
}

func ownerOf(t *testing.T, b *browser) string {
	t.Helper()
	var s struct{ User struct{ Person string } }
	b.ok("GET", "/api/session", nil, &s)
	return s.User.Person
}

func TestAResetLinkFollowsTheRoleLadderAndASpentOneEndsTheTargetsSessions(t *testing.T) {
	r := newAccountRig(t)
	admin, adminP := r.join("admin", "ann")
	_, memberP := r.join("member", "bob")
	owner, _ := r.signIn("me", "correct horse battery")
	ownerPerson := ownerOf(t, owner)

	t.Run("an Admin resets a Member only and nobody resets themselves", func(t *testing.T) {
		for name, target := range map[string]string{"an Owner": ownerPerson, "another Admin": adminP, "themselves": adminP} {
			if status, _ := admin.send("POST", "/api/people/"+target+"/reset", nil, nil); status != http.StatusForbidden {
				t.Errorf("an Admin's reset link for %s answered %d", name, status)
			}
		}
		if status, _ := owner.send("POST", "/api/people/"+ownerPerson+"/reset", nil, nil); status != http.StatusForbidden {
			t.Errorf("an Owner's reset link for themselves: %d", status)
		}
		_, data := r.people(admin)
		for _, p := range data.People {
			if want := p.Person == memberP; p.MayReset != want {
				t.Errorf("%s may_reset %v for an Admin", p.Username, p.MayReset)
			}
		}
	})

	t.Run("an Admin's link for a Member is replaced by the next and works once", func(t *testing.T) {
		var first, second linkOut
		admin.ok("POST", "/api/people/"+memberP+"/reset", nil, &first)
		admin.ok("POST", "/api/people/"+memberP+"/reset", nil, &second)
		stranger := r.browser()
		if status, _ := stranger.send("POST", "/api/invite/inspect", map[string]string{"token": first.token(t)}, nil); status != http.StatusGone {
			t.Fatalf("the replaced link: %d", status)
		}
		var facts console.LinkFacts
		stranger.ok("POST", "/api/invite/inspect", map[string]string{"token": second.token(t)}, &facts)
		if facts.Purpose != console.PurposeReset || facts.Username != "bob" || facts.InvitedBy != "ann" {
			t.Fatalf("facts %+v", facts)
		}
		bobBefore, _ := r.signIn("bob", "a brand new password")
		annOther, _ := r.signIn("ann", "a brand new password")
		var out struct{ CSRF string }
		stranger.ok("POST", "/api/invite/accept", map[string]string{"token": second.token(t), "username": "ignored", "password": "bob's new long password"}, &out)
		if isSignedIn(bobBefore) {
			t.Error("the target's older session survived the reset")
		}
		if !isSignedIn(annOther) || !isSignedIn(owner) {
			t.Error("another user's session ended with the reset")
		}
		stranger.csrf = out.CSRF
		var s struct {
			User struct{ Username, Person string }
		}
		stranger.ok("GET", "/api/session", nil, &s)
		if s.User.Username != "bob" || s.User.Person != memberP {
			t.Fatalf("the reset signed in %+v", s.User)
		}
		if _, status := r.signIn("bob", "a brand new password"); status != http.StatusUnauthorized {
			t.Errorf("the old password works: %d", status)
		}
		if _, status := r.signIn("bob", "bob's new long password"); status != http.StatusOK {
			t.Errorf("the new password does not work: %d", status)
		}
		if status, _ := stranger.send("POST", "/api/invite/accept", map[string]string{"token": second.token(t), "password": "bob's new long password"}, nil); status != http.StatusGone {
			t.Errorf("reusing a reset link: %d", status)
		}
	})

	t.Run("a reset link dies when its creator can no longer reset the target", func(t *testing.T) {
		var l linkOut
		admin.ok("POST", "/api/people/"+memberP+"/reset", nil, &l)
		if err := r.c.State.SetRole("owner", adminP, console.RoleMember); err != nil {
			t.Fatal(err)
		}
		if status, _ := r.browser().send("POST", "/api/invite/inspect", map[string]string{"token": l.token(t)}, nil); status != http.StatusGone {
			t.Fatalf("a link of a demoted creator: %d", status)
		}
	})
}

func TestAnOwnerRevokesLinksAndAnAdminOnlyTheirOwnResetLinks(t *testing.T) {
	r := newAccountRig(t)
	admin, _ := r.join("admin", "ann")
	_, memberP := r.join("member", "bob")
	owner, _ := r.signIn("me", "correct horse battery")
	var join, reset linkOut
	owner.ok("POST", "/api/invites", map[string]string{"role": "member"}, &join)
	admin.ok("POST", "/api/people/"+memberP+"/reset", nil, &reset)

	if status, _ := admin.send("DELETE", "/api/invites/"+join.ID, nil, nil); status != http.StatusForbidden {
		t.Errorf("an Admin revoking a join link: %d", status)
	}
	_, data := r.people(admin)
	if len(data.Invites) != 2 {
		t.Fatalf("the Admin sees %d links", len(data.Invites))
	}
	if status, _ := admin.send("DELETE", "/api/invites/nope", nil, nil); status != http.StatusNotFound {
		t.Errorf("an unknown link: %d", status)
	}
	admin.ok("DELETE", "/api/invites/"+reset.ID, nil, nil)
	owner.ok("DELETE", "/api/invites/"+join.ID, nil, nil)
	for _, l := range []linkOut{join, reset} {
		if status, _ := r.browser().send("POST", "/api/invite/accept", map[string]string{"token": l.token(t), "username": "zed", "password": "a brand new password"}, nil); status != http.StatusGone {
			t.Errorf("a revoked link: %d", status)
		}
	}
	if _, data := r.people(owner); len(data.Invites) != 0 {
		t.Errorf("links left: %+v", data.Invites)
	}
}

func TestTenWrongLinksAMinuteSlowEveryoneDownAndTwentyLinksWaitAtMost(t *testing.T) {
	r := newAccountRig(t)
	owner, _ := r.signIn("me", "correct horse battery")
	stranger := r.browser()
	for range 10 {
		stranger.send("POST", "/api/invite/inspect", map[string]string{"token": "wrong"}, nil)
	}
	var live linkOut
	owner.ok("POST", "/api/invites", map[string]string{"role": "member"}, &live)
	if status, raw := stranger.send("POST", "/api/invite/inspect", map[string]string{"token": live.token(t)}, nil); status != http.StatusTooManyRequests || !strings.Contains(string(raw), "slow_down") {
		t.Fatalf("after ten failures: %d %s", status, raw)
	}

	r2 := newAccountRig(t)
	owner2, _ := r2.signIn("me", "correct horse battery")
	for range 20 {
		owner2.ok("POST", "/api/invites", map[string]string{"role": "member"}, nil)
	}
	if status, raw := owner2.send("POST", "/api/invites", map[string]string{"role": "member"}, nil); status != http.StatusConflict || !strings.Contains(string(raw), "too_many_links") {
		t.Fatalf("the twenty-first link: %d %s", status, raw)
	}
}

func TestOneUsernamesWrongPasswordsDoNotLockAnotherUserOut(t *testing.T) {
	r := newAccountRig(t)
	r.join("member", "bob")
	for range 5 {
		r.signIn("bob", "wrong wrong wrong")
	}
	if _, status := r.signIn("bob", "a brand new password"); status != http.StatusTooManyRequests {
		t.Errorf("a sixth try for the same name: %d", status)
	}
	if _, status := r.signIn("me", "correct horse battery"); status != http.StatusOK {
		t.Errorf("another name is locked out: %d", status)
	}
	if _, status := r.signIn("BOB", "a brand new password"); status != http.StatusTooManyRequests {
		t.Errorf("the same name in another case: %d", status)
	}
}

func TestTheSessionCookieIsSecureOnlyForAnHTTPSConsole(t *testing.T) {
	r := newAccountRig(t)
	secure := func() bool {
		b := r.browser()
		body, _ := json.Marshal(map[string]string{"username": "me", "password": "correct horse battery"})
		req, _ := http.NewRequest("POST", r.srv.URL+"/api/login", strings.NewReader(string(body)))
		req.Header.Set("Origin", r.srv.URL)
		req.Header.Set("Content-Type", "application/json")
		resp, err := b.client.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		resp.Body.Close()
		for _, c := range resp.Cookies() {
			if c.Name == "cn_console" {
				return c.Secure
			}
		}
		t.Fatal("no session cookie")
		return false
	}
	if secure() {
		t.Error("an http console sets a Secure cookie that the browser would drop")
	}
	r.c.Sessions.Secure = true
	if !secure() {
		t.Error("an https console's cookie is not Secure")
	}
}
