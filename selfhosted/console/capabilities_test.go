package console

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"
)

func routedConsole(t *testing.T) (*Console, *router) {
	t.Helper()
	c, _ := recordedConsole(t)
	addUser(t, c.State, "p-ada", "ada", RoleAdmin)
	rt := &router{c: c, mux: http.NewServeMux()}
	c.routes(rt)
	return c, rt
}

func signedInRequest(t *testing.T, c *Console, person, method, path string) *http.Request {
	t.Helper()
	rec := httptest.NewRecorder()
	csrf, err := c.Sessions.Start(rec, person)
	if err != nil {
		t.Fatal(err)
	}
	r := httptest.NewRequest(method, path, strings.NewReader("{}"))
	r.Header.Set("Content-Type", "application/json")
	r.Header.Set(csrfHeader, csrf)
	r.AddCookie(rec.Result().Cookies()[0])
	return r
}

func filled(pattern string) (string, string) {
	method, path, _ := strings.Cut(pattern, " ")
	return method, regexp.MustCompile(`\{[^}]+\}`).ReplaceAllString(path, "x")
}

func TestEveryRegisteredRouteDeclaresACapabilityOrIsPublic(t *testing.T) {
	_, rt := routedConsole(t)
	for _, p := range rt.patterns {
		_, declared := routeCapabilities[p]
		public := slices.Contains(publicRoutes, p)
		if declared == public {
			t.Errorf("route %q is declared %v and public %v: give it exactly one", p, declared, public)
		}
	}
	for p := range routeCapabilities {
		if !slices.Contains(rt.patterns, p) {
			t.Errorf("the capability table names %q, which no route registers", p)
		}
	}
	for _, p := range firstRunRoutes {
		if _, ok := routeCapabilities[p]; !ok {
			t.Errorf("the first-run route %q declares no capability for after the first run", p)
		}
	}
}

func TestNoRouteIsRegisteredOnTheConsoleMuxPastTheRouter(t *testing.T) {
	files, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatal(err)
	}
	direct := regexp.MustCompile(`\.Handle(Func)?\(`)
	for _, f := range files {
		if strings.HasSuffix(f, "_test.go") || f == "capabilities.go" || f == "host.go" {
			continue
		}
		raw, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		if loc := direct.FindIndex(raw); loc != nil {
			t.Errorf("%s registers a route at byte %d without the router: use router.handle", f, loc[0])
		}
	}
}

func TestARouteMissingFromTheTableIsRefusedForEveryone(t *testing.T) {
	c, _ := routedConsole(t)
	h := c.gate("GET /api/undeclared", func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) })
	w := httptest.NewRecorder()
	h(w, signedInRequest(t, c, "owner", http.MethodGet, "/api/undeclared"))
	if w.Code != http.StatusForbidden {
		t.Fatalf("an undeclared route answered %d", w.Code)
	}
}

func TestEveryRouteRefusesEveryRoleWithoutItsCapability(t *testing.T) {
	c, rt := routedConsole(t)
	roles := map[Role]string{RoleOwner: "owner", RoleAdmin: "p-ada", RoleMember: "p-bob"}
	refused := 0
	for pattern, need := range routeCapabilities {
		for role, person := range roles {
			if (User{Role: role}).Can(need) {
				continue
			}
			t.Run(string(role)+" "+pattern, func(t *testing.T) {
				method, path := filled(pattern)
				w := httptest.NewRecorder()
				rt.mux.ServeHTTP(w, signedInRequest(t, c, person, method, path))
				var body apiError
				_ = json.Unmarshal(w.Body.Bytes(), &body)
				if w.Code != http.StatusForbidden || body.Code != "forbidden" {
					t.Fatalf("a %s got %d %s", role, w.Code, w.Body)
				}
			})
			refused++
		}
	}
	if refused == 0 {
		t.Fatal("no route refused any role")
	}
}

func TestTheRolesHoldTheCapabilitiesOfTheTable(t *testing.T) {
	want := map[Role][]Capability{
		RoleMember: {CapAccountOwn, CapActivityOwn, CapApprovalsOwn, CapKeysOwn, CapNotificationsOwn, CapSigninOwn},
		RoleAdmin: {CapAccountOwn, CapActivityAny, CapActivityOwn, CapActivitySettings, CapApprovalsAny, CapApprovalsOwn, CapConnectorsManage,
			CapKeysAny, CapKeysOwn, CapNotificationsOwn, CapPeopleRead, CapPeopleReset, CapSetupManage, CapSigninOwn, CapSkillsManage},
	}
	for role, caps := range want {
		if got := (User{Role: role}).Capabilities(); !slices.Equal(got, caps) {
			t.Errorf("%s holds %v, want %v", role, got, caps)
		}
	}
	if got := (User{Role: RoleOwner}).Capabilities(); len(got) != len(capabilityRole) {
		t.Errorf("an owner holds %d of %d capabilities", len(got), len(capabilityRole))
	}
	if (User{Role: "guest"}).Can(CapAccountOwn) || (User{Role: RoleOwner}).Can("keys.everything") {
		t.Error("an unknown role or capability was allowed")
	}
}

func TestMayLetsTheOwnerOfARowOrAHolderOfTheAnyCapability(t *testing.T) {
	bob, ada := User{Person: "p-bob", Role: RoleMember}, User{Person: "p-ada", Role: RoleAdmin}
	switch {
	case !may(bob, CapKeysOwn, CapKeysAny, "p-bob"):
		t.Error("a member may not act on their own key")
	case may(bob, CapKeysOwn, CapKeysAny, "p-ada"):
		t.Error("a member may act on another's key")
	case may(bob, CapKeysOwn, CapKeysAny, ""):
		t.Error("a member may act on a key without an owner")
	case !may(ada, CapKeysOwn, CapKeysAny, "p-bob"):
		t.Error("an admin may not act on a member's key")
	}
}
