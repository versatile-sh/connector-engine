package console_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/versatile-sh/connector-engine/port"
	"github.com/versatile-sh/connector-engine/selfhosted/console"
	"github.com/versatile-sh/connector-engine/selfhosted/surface"
)

func (r *rig) member(person, username string) *browser {
	r.t.Helper()
	console.AddTestUser(r.t, r.c.State, person, username, ownerPassword, console.RoleMember)
	if err := r.c.ReconcileRoles(context.Background()); err != nil {
		r.t.Fatal(err)
	}
	jar, _ := cookiejar.New(nil)
	b := &browser{t: r.t, base: r.base, client: &http.Client{Jar: jar}}
	var out struct{ CSRF string }
	b.ok("POST", "/api/login", map[string]string{"username": username, "password": ownerPassword}, &out)
	b.csrf = out.CSRF
	return b
}

func (r *rig) decidedBy(id string) string {
	r.t.Helper()
	var d struct{ Approval port.Approval }
	if err := r.eng.Read(context.Background(), http.MethodGet, "/v1/approvals/"+id, nil, &d); err != nil {
		r.t.Fatal(err)
	}
	return d.Approval.DecidedBy
}

func (r *rig) channel(b *browser, ownerID string) console.ChannelView {
	r.t.Helper()
	var view console.ChannelView
	b.ok("POST", "/api/notifications/channels", map[string]any{"kind": "telegram", "settings": map[string]string{"token": "123456:ABCDEFGHIJKLMNOPQRST", "owner_user_id": ownerID}}, &view)
	return view
}

func quiet[T any](t *testing.T, ch <-chan T, what string) {
	t.Helper()
	select {
	case v := <-ch:
		t.Fatalf("%s got %+v", what, v)
	default:
	}
}

func TestAnApprovalReachesOnlyTheKeyOwnersChannelAndAnswersAsThatOwner(t *testing.T) {
	r := newRig(t)
	alice, bob := r.member("p-alice", "alice"), r.member("p-bob", "bob")
	r.channel(alice, "1")
	r.channel(bob, "2")
	aliceChat, bobChat := r.fakes["1"], r.fakes["2"]
	within(t, aliceChat.listened, 5*time.Second)
	within(t, bobChat.listened, 5*time.Second)
	var tok console.AgentToken
	bob.ok("POST", "/api/agents", map[string]any{"name": "bobkey"}, &tok)
	pending := r.call(r.connectAs(tok, "", nil), "create", map[string]any{"title": "bob's"})
	p := within(t, bobChat.prompts, 8*time.Second)
	if _, err := aliceChat.press(context.Background(), surface.Click{Key: p.Key, Choice: surface.ChoiceOnce, User: "1"}); err == nil || !strings.Contains(err.Error(), "not yours to answer") {
		t.Fatalf("alice's chat answering bob's call: %v", err)
	}
	o, err := bobChat.press(context.Background(), surface.Click{Key: p.Key, Choice: surface.ChoiceOnce, User: "2"})
	if err != nil || o.State != surface.StateApproved {
		t.Fatalf("bob's click %+v %v", o, err)
	}
	ran(t, within(t, pending, 8*time.Second), "create")
	within(t, bobChat.settled, 5*time.Second)
	quiet(t, aliceChat.prompts, "alice's chat")
	quiet(t, r.fake.prompts, "the first owner's chat")
	if by := r.decidedBy(p.Approval); by != "p-bob" {
		t.Fatalf("decided by %q", by)
	}
}

func TestADesktopLinkDecidesOnlyAsItsDevicesOwner(t *testing.T) {
	r := newRig(t)
	alice, bob := r.member("p-alice", "alice"), r.member("p-bob", "bob")
	pair := func(b *browser) (string, string, <-chan console.HubEvent) {
		var paired struct {
			Key    string
			Device console.DeviceView
		}
		b.ok("POST", "/api/notifications/devices", map[string]any{"name": "laptop"}, &paired)
		return paired.Device.ID, paired.Key, streamEvents(t, r.base, paired.Key)
	}
	aliceDevice, aliceKey, aliceEvents := pair(alice)
	_, bobKey, bobEvents := pair(bob)
	var tok console.AgentToken
	bob.ok("POST", "/api/agents", map[string]any{"name": "bobkey"}, &tok)
	pending := r.call(r.connectAs(tok, "", nil), "create", map[string]any{"title": "bob's"})
	ev := within(t, bobEvents, 8*time.Second)
	if ev.Type != "prompt" {
		t.Fatalf("event %+v", ev)
	}
	aliceLate := streamEvents(t, r.base, aliceKey)
	if late := within(t, streamEvents(t, r.base, bobKey), 5*time.Second); late.Approval != ev.Approval {
		t.Fatalf("bob's late stream replayed %+v", late)
	}
	quiet(t, aliceLate, "alice's late stream")
	anon := &browser{t: t, base: r.base, client: &http.Client{}}
	forged, err := r.c.LinkToken(ev.Approval, aliceDevice, time.Now().Add(time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	status, raw := anon.send("POST", "/api/approvals/"+ev.Approval+"/decide", map[string]any{"choice": "once", "token": forged}, nil)
	var refusal struct{ Error, Code string }
	if status != http.StatusForbidden || json.Unmarshal(raw, &refusal) != nil || refusal.Code != "not_an_approver" || !strings.Contains(refusal.Error, "not yours to answer") {
		t.Fatalf("alice's device deciding bob's call: %d %s", status, raw)
	}
	link, _ := url.Parse(ev.Link)
	if status, raw := anon.send("POST", "/api/approvals/"+ev.Approval+"/decide", map[string]any{"choice": "once", "token": link.Query().Get("t")}, nil); status != http.StatusOK {
		t.Fatalf("bob's device: %d %s", status, raw)
	}
	ran(t, within(t, pending, 8*time.Second), "create")
	if ev := within(t, bobEvents, 5*time.Second); ev.Type != "settled" {
		t.Fatalf("settled %+v", ev)
	}
	quiet(t, aliceEvents, "alice's device")
	if by := r.decidedBy(ev.Approval); by != "p-bob" {
		t.Fatalf("decided by %q", by)
	}
}

func TestAUserCannotSeeOrChangeAnotherUsersChannelsDevicesOrApps(t *testing.T) {
	r := newRig(t)
	alice, bob := r.member("p-alice", "alice"), r.member("p-bob", "bob")
	ch := r.channel(bob, "2")
	var paired struct{ Device console.DeviceView }
	bob.ok("POST", "/api/notifications/devices", map[string]any{"name": "laptop"}, &paired)
	bob.ok("POST", "/api/agents", map[string]any{"name": "bobkey"}, nil)
	var list struct {
		Channels []console.ChannelView
		Devices  []console.DeviceView
		Apps     []struct{ Name string }
	}
	alice.ok("GET", "/api/notifications", nil, &list)
	if len(list.Channels)+len(list.Devices)+len(list.Apps) != 0 {
		t.Fatalf("alice sees %+v", list)
	}
	for _, req := range []struct{ method, path string }{
		{"POST", "/api/notifications/channels/" + ch.ID + "/test"},
		{"DELETE", "/api/notifications/channels/" + ch.ID},
		{"PATCH", "/api/notifications/devices/" + paired.Device.ID},
		{"DELETE", "/api/notifications/devices/" + paired.Device.ID},
		{"POST", "/api/notifications/devices"},
		{"PUT", "/api/notifications/apps/bobkey"},
	} {
		body := map[string]any{"name": "mine now", "id": paired.Device.ID, "elicit": true}
		if status, raw := alice.send(req.method, req.path, body, nil); status != http.StatusNotFound {
			t.Errorf("%s %s: %d %s", req.method, req.path, status, raw)
		}
	}
	bob.ok("GET", "/api/notifications", nil, &list)
	if len(list.Channels) != 1 || list.Channels[0].SecretHint != "QRST" || len(list.Devices) != 1 || list.Devices[0].Name != "laptop" || len(list.Apps) != 1 || list.Apps[0].Name != "bobkey" {
		t.Fatalf("bob's own settings after alice tried: %+v", list)
	}
	var tested struct{ OK bool }
	bob.ok("POST", "/api/notifications/channels/"+ch.ID+"/test", nil, &tested)
	if !tested.OK {
		t.Fatal("bob's channel no longer works")
	}
}

func TestTheNotificationsListServesEveryChannelKindWithItsFields(t *testing.T) {
	r := newRig(t)
	alice := r.member("p-alice", "alice")
	var list struct {
		Kinds []struct {
			Kind        string
			Title       string
			Interactive bool
			Fields      []struct {
				Name     string
				Secret   bool
				Optional bool
			}
		}
	}
	alice.ok("GET", "/api/notifications", nil, &list)
	byKind := map[string]bool{}
	for _, k := range list.Kinds {
		byKind[k.Kind] = k.Interactive
		if k.Title == "" || len(k.Fields) == 0 {
			t.Errorf("kind %s is served without a title or fields", k.Kind)
		}
	}
	if len(byKind) != 6 || !byKind["slack"] || byKind["ntfy"] {
		t.Fatalf("kinds served: %v", byKind)
	}
}
