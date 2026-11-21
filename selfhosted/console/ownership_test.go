package console_test

import (
	"context"
	"encoding/json"
	"net/http"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/versatile-sh/connector-engine/port"
	"github.com/versatile-sh/connector-engine/selfhosted/console"
)

type ownedKey struct {
	Name  string           `json:"name"`
	Owner console.OwnerRef `json:"owner"`
}

func (p profiled) keys(query string) []ownedKey {
	p.t.Helper()
	var out struct{ Agents []ownedKey }
	p.b.ok("GET", "/api/agents"+query, nil, &out)
	return out.Agents
}

func (p profiled) approvalRows(query string) []console.ApprovalRow {
	p.t.Helper()
	var out struct{ Approvals []console.ApprovalRow }
	p.b.ok("GET", "/api/approvals"+query, nil, &out)
	return out.Approvals
}

func (p profiled) activityUntil(query string, ok func(console.ActivityPage) bool) console.ActivityPage {
	p.t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	var page console.ActivityPage
	for time.Now().Before(deadline) {
		page = console.ActivityPage{}
		p.b.ok("GET", "/api/activity"+query, nil, &page)
		if ok(page) {
			return page
		}
		time.Sleep(50 * time.Millisecond)
	}
	p.t.Fatalf("activity %s never matched: %+v", query, page.Calls)
	return page
}

func names(keys []ownedKey) []string {
	out := []string{}
	for _, k := range keys {
		out = append(out, k.Name+"@"+k.Owner.Username)
	}
	return out
}

func codeOf(raw []byte) string {
	var e struct{ Code string }
	_ = json.Unmarshal(raw, &e)
	return e.Code
}

func (p profiled) refusedAs(t *testing.T, want int, method, path string, body any) {
	t.Helper()
	if status, raw := p.b.send(method, path, body, nil); status != want {
		t.Errorf("%s %s answered %d %s, want %d", method, path, status, raw, want)
	}
}

func TestAMemberSeesAndActsOnOnlyTheirOwnKeysApprovalsAndActivity(t *testing.T) {
	p := profiledStack(t)
	bob := profiled{stack: p.signInAs("p-bob", "bob", console.RoleMember), conn: p.conn}
	alpha := p.mint("alpha")
	bobkey := bob.mint("bobkey")

	t.Run("keys are scoped to their owner unless everyone is asked by a manager", func(t *testing.T) {
		if got := names(bob.keys("")); !slices.Equal(got, []string{"bobkey@bob"}) {
			t.Errorf("bob's keys %v", got)
		}
		if got := names(bob.keys("?scope=everyone")); !slices.Equal(got, []string{"bobkey@bob"}) {
			t.Errorf("bob's keys for everyone %v", got)
		}
		if got := names(p.keys("")); !slices.Equal(got, []string{"alpha@me"}) {
			t.Errorf("the owner's keys %v", got)
		}
		if got := names(p.keys("?scope=everyone")); !slices.Equal(got, []string{"alpha@me", "bobkey@bob"}) {
			t.Errorf("everyone's keys %v", got)
		}
		if k := p.keys("?scope=everyone")[1]; k.Owner.Person != "p-bob" {
			t.Errorf("bobkey's owner %+v", k.Owner)
		}
		p.refusedAs(t, http.StatusBadRequest, "GET", "/api/agents?scope=all", nil)
	})

	t.Run("another person's key answers like a missing one", func(t *testing.T) {
		for _, name := range []string{"alpha", "nosuch"} {
			bob.refusedAs(t, http.StatusNotFound, "DELETE", "/api/agents/"+name, nil)
			bob.refusedAs(t, http.StatusNotFound, "GET", "/api/connectors/"+p.conn+"/profiles/"+name, nil)
			bob.refusedAs(t, http.StatusNotFound, "PUT", "/api/connectors/"+p.conn+"/profiles/"+name, map[string]any{"names": []string{"lookup"}, "access": "off"})
			bob.refusedAs(t, http.StatusNotFound, "PUT", "/api/notifications/apps/"+name, map[string]any{"elicit": true})
		}
		if got := bob.batch("PUT", "/api/connectors/"+p.conn+"/profiles", map[string]any{"mode": "off", "keys": []string{"alpha"}}); got["alpha"] != "not_found" {
			t.Errorf("a batch over another's key %v", got)
		}
		if pr := bob.summary().Profiles; len(pr) != 1 || pr[0].Agent != "bobkey" {
			t.Errorf("bob's connector shows the profiles %+v", pr)
		}
		for _, path := range []string{"/api/connectors", "/api/skills", "/api/notifications", "/api/slots", "/api/agents?scope=everyone"} {
			if _, raw := bob.b.send("GET", path, nil, nil); strings.Contains(string(raw), `"alpha"`) {
				t.Errorf("%s shows bob another person's key: %s", path, raw)
			}
		}
		if !callsOK(alpha, "lookup") {
			t.Fatal("bob's refused acts changed alpha")
		}
	})

	t.Run("a manager narrows a member's key and its owner stays", func(t *testing.T) {
		p.narrow("bobkey", "lookup", console.AccessAsk)
		var binds struct{ Items []port.Binding }
		if err := p.c.Engine.Read(context.Background(), http.MethodGet, "/v1/connectors/"+p.conn+"/bindings", nil, &binds); err != nil {
			t.Fatal(err)
		}
		if i := slices.IndexFunc(binds.Items, func(b port.Binding) bool { return b.Agent == "bobkey" }); i < 0 || binds.Items[i].AgentOwner != "p-bob" {
			t.Fatalf("bobkey's binding after the owner narrowed it %+v", binds.Items)
		}
	})

	p.narrow("alpha", "lookup", console.AccessAsk)
	ran := make(chan bool, 2)
	go func() { ran <- callsOK(alpha, "lookup") }()
	go func() { ran <- callsOK(bobkey, "lookup") }()
	theirs, mine := p.heldFor("alpha"), bob.heldFor("bobkey")

	t.Run("approvals are scoped and another person's is not found", func(t *testing.T) {
		if rows := bob.approvalRows("?scope=everyone"); len(rows) != 1 || rows[0].ID != port.ApprovalID(mine) || rows[0].Owner.Username != "bob" {
			t.Errorf("bob's approvals %+v", rows)
		}
		if rows := bob.approvalRows(""); len(rows) != 1 || slices.Contains(rows[0].Choices, "always") || !slices.Contains(rows[0].Choices, "once") {
			t.Errorf("bob's approval offers %+v", rows)
		}
		if rows := p.approvalRows(""); len(rows) != 1 || rows[0].ID != port.ApprovalID(theirs) || !slices.Contains(rows[0].Choices, "always") {
			t.Errorf("the owner's approvals %+v", rows)
		}
		if rows := p.approvalRows("?scope=everyone"); len(rows) != 2 {
			t.Errorf("everyone's approvals %+v", rows)
		}
		bob.refusedAs(t, http.StatusNotFound, "GET", "/api/approvals/"+theirs, nil)
		bob.refusedAs(t, http.StatusNotFound, "POST", "/api/approvals/"+theirs, map[string]any{"approve": true})
		bob.refusedAs(t, http.StatusNotFound, "POST", "/api/approvals/"+theirs+"/decide", map[string]any{"choice": "once"})
	})

	t.Run("always is offered only with keys.any and refused for a member", func(t *testing.T) {
		var own, managed console.ApprovalView
		bob.b.ok("GET", "/api/approvals/"+mine, nil, &own)
		p.b.ok("GET", "/api/approvals/"+theirs, nil, &managed)
		if slices.Contains(own.Choices, "always") || !slices.Contains(own.Choices, "once") {
			t.Errorf("bob is offered %v", own.Choices)
		}
		if !slices.Contains(managed.Choices, "always") || managed.Owner.Username != "me" {
			t.Errorf("the owner is offered %v for %+v", managed.Choices, managed.Owner)
		}
		status, raw := bob.b.send("POST", "/api/approvals/"+mine+"/decide", map[string]any{"choice": "always"}, nil)
		if status != http.StatusForbidden || codeOf(raw) != "forbidden" {
			t.Errorf("bob's always answered %d %s", status, raw)
		}
	})

	t.Run("always is refused for a member through a desktop link", func(t *testing.T) {
		if err := p.c.State.PutDevice(console.Device{ID: "d1", Name: "laptop", Owner: "p-bob", KeyHash: "unused", CreatedAt: time.Now()}); err != nil {
			t.Fatal(err)
		}
		link := func() string {
			tok, err := p.c.LinkToken(mine, "d1", time.Now().Add(time.Hour))
			if err != nil {
				t.Fatal(err)
			}
			return tok
		}
		anon := &browser{t: t, base: p.base, client: &http.Client{}}
		var page console.ApprovalView
		anon.ok("GET", "/api/approvals/"+mine+"?t="+link(), nil, &page)
		if slices.Contains(page.Choices, "always") {
			t.Errorf("the link page offers %v", page.Choices)
		}
		status, raw := anon.send("POST", "/api/approvals/"+mine+"/decide", map[string]any{"choice": "always", "token": link()}, nil)
		if status != http.StatusForbidden {
			t.Errorf("always through a link answered %d %s", status, raw)
		}
		anon.ok("POST", "/api/approvals/"+mine+"/decide", map[string]any{"choice": "once", "token": link()}, nil)
	})

	p.b.ok("POST", "/api/approvals/"+theirs+"/decide", map[string]any{"choice": "once"}, nil)
	for range 2 {
		if !<-ran {
			t.Fatal("an approved call did not run")
		}
	}

	t.Run("recent decisions are the viewer's own unless a manager asks for everyone", func(t *testing.T) {
		decisions := func(who profiled, query string) []console.DecisionRow {
			var out struct{ Decisions []console.DecisionRow }
			who.b.ok("GET", "/api/approvals/recent"+query, nil, &out)
			return out.Decisions
		}
		ids := func(rows []console.DecisionRow) []string {
			out := []string{}
			for _, row := range rows {
				out = append(out, row.ID)
			}
			slices.Sort(out)
			return out
		}
		if got := decisions(bob, ""); !slices.Equal(ids(got), []string{mine}) || got[0].Owner.Person != "p-bob" || got[0].State == "pending" {
			t.Errorf("bob's decisions %+v", got)
		}
		if got := decisions(bob, "?scope=everyone"); !slices.Equal(ids(got), []string{mine}) {
			t.Errorf("bob asking for everyone gets %+v", got)
		}
		if got := decisions(p, ""); !slices.Equal(ids(got), []string{theirs}) {
			t.Errorf("the owner's decisions %+v", got)
		}
		both := []string{mine, theirs}
		slices.Sort(both)
		if got := decisions(p, "?scope=everyone"); !slices.Equal(ids(got), both) {
			t.Errorf("everyone's decisions %+v", got)
		}
		p.refusedAs(t, http.StatusBadRequest, "GET", "/api/approvals/recent?scope=all", nil)
	})

	t.Run("activity is scoped by the person a capture was made for", func(t *testing.T) {
		mineOnly := func(person string) func(console.ActivityPage) bool {
			return func(pg console.ActivityPage) bool {
				return len(pg.Calls) > 0 && !slices.ContainsFunc(pg.Calls, func(c console.ActivityCall) bool { return c.Owner.Person != person })
			}
		}
		bobs := bob.activityUntil("?scope=everyone", mineOnly("p-bob"))
		owners := p.activityUntil("", mineOnly("owner"))
		p.activityUntil("?scope=everyone&person=p-bob", mineOnly("p-bob"))
		p.activityUntil("?scope=everyone", func(pg console.ActivityPage) bool { return len(pg.Calls) == len(bobs.Calls)+len(owners.Calls) })
		if bobs.Calls[0].Owner.Username != "bob" {
			t.Errorf("bob's row owner %+v", bobs.Calls[0].Owner)
		}
		bob.refusedAs(t, http.StatusNotFound, "GET", "/api/activity/"+owners.Calls[0].ID, nil)
		var d console.ActivityDetail
		p.b.ok("GET", "/api/activity/"+bobs.Calls[0].ID, nil, &d)
		if d.Owner.Person != "p-bob" {
			t.Errorf("the owner reads bob's call as %+v", d.Owner)
		}
	})
}

func TestTheSessionNamesTheUserTheirCapabilitiesAndThePeopleCount(t *testing.T) {
	p := profiledStack(t)
	bob := p.signInAs("p-bob", "bob", console.RoleMember)
	var s struct {
		User struct {
			Person, Username string
			Role             console.Role
		}
		Can    []string
		People int
	}
	bob.b.ok("GET", "/api/session", nil, &s)
	if s.User.Person != "p-bob" || s.User.Username != "bob" || s.User.Role != console.RoleMember || s.People != 2 {
		t.Fatalf("bob's session %+v", s)
	}
	if want := []string{"account.own", "activity.own", "approvals.own", "keys.own", "notifications.own", "signin.own"}; !slices.Equal(s.Can, want) {
		t.Fatalf("bob can %v", s.Can)
	}
	p.b.ok("GET", "/api/session", nil, &s)
	if !slices.Contains(s.Can, "people.manage") || s.User.Role != console.RoleOwner {
		t.Fatalf("the owner's session %+v", s)
	}
}

func TestAMemberAppliesASlotToTheirOwnKeyButNeverToAnothersNorDiscardsOne(t *testing.T) {
	dst := importedSlots(t, slotSetup(t), "mine")
	bob := profiled{stack: dst.signInAs("p-bob", "bob", console.RoleMember), conn: dst.conn}
	bob.mint("bobkey")
	bob.refusedAs(t, http.StatusNotFound, "POST", "/api/slots/alpha/claim", map[string]any{"key": "mine"})
	bob.refusedAs(t, http.StatusForbidden, "DELETE", "/api/slots/beta", nil)
	if len(bob.slots()) != 3 {
		t.Fatal("a refused claim or discard used a slot")
	}
	bob.b.ok("POST", "/api/slots/alpha/claim", map[string]any{"key": "bobkey"}, nil)
	dst.hasExactly("bobkey", console.ProfileCustom, map[string]console.Access{"lookup": console.AccessAsk})
	if a, _ := dst.c.State.Agent("bobkey"); a.Owner != "p-bob" {
		t.Fatalf("the claimed key's owner %q", a.Owner)
	}
	dst.b.ok("POST", "/api/slots/gamma/claim", map[string]any{"key": "bobkey"}, nil)
	dst.b.ok("DELETE", "/api/slots/beta", nil, nil)
}

func TestABrowserConnectedKeyIsReusedOnlyByItsOwnUser(t *testing.T) {
	p := profiledStack(t)
	bob := profiled{stack: p.signInAs("p-bob", "bob", console.RoleMember), conn: p.conn}
	ann := profiled{stack: p.signInAs("p-ann", "ann", console.RoleMember), conn: p.conn}
	client := p.register()
	first := p.consentWith(client, "")
	if again := p.consentWith(client, ""); again != first {
		t.Fatalf("the owner's reconnect made %q, not %q", again, first)
	}
	bobs, anns := bob.consentWith(client, ""), ann.consentWith(client, "")
	if bobs == first || anns == first || anns == bobs {
		t.Fatalf("one key was shared: %q %q %q", first, bobs, anns)
	}
	for name, owner := range map[string]string{first: "owner", bobs: "p-bob", anns: "p-ann"} {
		if a, _ := p.c.State.Agent(name); a.Owner != owner {
			t.Errorf("%s belongs to %q, want %q", name, a.Owner, owner)
		}
	}
	if again := bob.consentWith(client, ""); again != bobs {
		t.Fatalf("bob's reconnect made %q, not %q", again, bobs)
	}
}

func TestAnAdminHandlesSetupFilesButNeverAFullBackup(t *testing.T) {
	p := profiledStack(t)
	full := p.export()
	ada := p.signInAs("p-ada", "ada", console.RoleAdmin)
	if status, raw := ada.b.send("POST", "/api/backup/export", map[string]any{"mode": "full", "password": ownerPassword, "passphrase": passphrase, "confirm": true}, nil); status != http.StatusForbidden || codeOf(raw) != "forbidden" {
		t.Errorf("an admin's full export answered %d %s", status, raw)
	}
	if status, raw := ada.b.send("POST", "/api/backup/export", map[string]any{"mode": "setup"}, nil); status != http.StatusOK {
		t.Errorf("an admin's setup export answered %d %s", status, raw)
	}
	for _, path := range []string{"/api/backup/inspect", "/api/backup/import"} {
		if status, raw := ada.upload(path, full, passphrase); status != http.StatusForbidden {
			t.Errorf("an admin's %s of a full backup answered %d %s", path, status, raw)
		}
	}
	setup := p.exportSetup()
	for _, path := range []string{"/api/backup/inspect", "/api/backup/import"} {
		if status, raw := ada.upload(path, setup, ""); status != http.StatusOK {
			t.Errorf("an admin's %s of a setup answered %d %s", path, status, raw)
		}
	}
}
