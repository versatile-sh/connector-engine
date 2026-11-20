package admin_test

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/versatile-sh/connector-engine/cli/admin"
	"github.com/versatile-sh/connector-engine/server"
)

type seen struct {
	Method, Path, Query, Auth, Key string
	Body                           map[string]any
}

func engine(t *testing.T, status int, reply string) (*httptest.Server, *seen) {
	t.Helper()
	got := &seen{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		data, _ := io.ReadAll(r.Body)
		*got = seen{Method: r.Method, Path: r.URL.Path, Query: r.URL.RawQuery, Auth: r.Header.Get("Authorization"), Key: r.Header.Get("Idempotency-Key")}
		_ = json.Unmarshal(data, &got.Body)
		w.WriteHeader(status)
		_, _ = w.Write([]byte(reply))
	}))
	t.Cleanup(srv.Close)
	return srv, got
}

func run(srv *httptest.Server, args ...string) (int, string, string) {
	var out, errOut bytes.Buffer
	cmd := admin.Command(&out, &errOut, []string{"VSH_ADMIN_URL=" + srv.URL, "VSH_ADMIN_TOKEN=tok"})
	code := cmd.Run(context.Background(), args)
	return code, out.String(), errOut.String()
}

func TestCommandsBecomeTheRequestsOfTheTable(t *testing.T) {
	srv, got := engine(t, 200, `{"ok":true}`)
	for name, c := range map[string]struct {
		args  []string
		want  seen
		field string
	}{
		"query":          {[]string{"listings", "list", "--workspace", "w1", "--q", "git"}, seen{Method: "GET", Path: "/v1/listings", Query: "q=git&workspace=w1"}, ""},
		"path value":     {[]string{"connectors", "get", "c1"}, seen{Method: "GET", Path: "/v1/connectors/c1"}, ""},
		"positional":     {[]string{"approvals", "decide", "a1", "approve", "--note", "fine"}, seen{Method: "POST", Path: "/v1/approvals/a1:decide"}, "decision"},
		"flags after":    {[]string{"tenants", "create", "t9"}, seen{Method: "PUT", Path: "/v1/tenants/t9"}, ""},
		"body from json": {[]string{"grants", "create", "--json", `{"selector":"*","mode":"allow"}`, "--scope-kind", "agent"}, seen{Method: "POST", Path: "/v1/grants"}, "scope_kind"},
		"delete":         {[]string{"grants", "delete", "g1"}, seen{Method: "DELETE", Path: "/v1/grants/g1"}, ""},
		"generic call":   {[]string{"call", "listPauses"}, seen{Method: "GET", Path: "/v1/pauses"}, ""},
		"sources sync":   {[]string{"sources", "sync", "official"}, seen{Method: "POST", Path: "/v1/sources/official:sync"}, ""},
	} {
		if code, _, errOut := run(srv, c.args...); code != 0 {
			t.Fatalf("%s: exit %d %s", name, code, errOut)
		}
		if got.Method != c.want.Method || got.Path != c.want.Path || got.Query != c.want.Query || got.Auth != "Bearer tok" {
			t.Errorf("%s: %+v, want %+v", name, got, c.want)
		}
		if (c.want.Method != "GET") != (got.Key != "") {
			t.Errorf("%s: idempotency key %q on %s", name, got.Key, c.want.Method)
		}
		if c.field != "" && got.Body[c.field] == nil {
			t.Errorf("%s: body %v lacks %s", name, got.Body, c.field)
		}
	}
	run(srv, "approvals", "decide", "a1", "deny", "--note", "no")
	if got.Body["decision"] != "deny" || got.Body["note"] != "no" {
		t.Errorf("decide body: %v", got.Body)
	}
}

func TestAnErrorPrintsItsCodeAndFailsTheCommand(t *testing.T) {
	srv, _ := engine(t, 403, `{"code":"permission_denied","detail":"ana may not tenant.create on installation"}`)
	code, _, errOut := run(srv, "tenants", "create", "t9")
	if code != 1 || !strings.Contains(errOut, "403 permission_denied: ana may not") {
		t.Fatalf("%d %q", code, errOut)
	}
	if code, _, _ := run(srv, "nope", "nothing"); code != 2 {
		t.Errorf("an unknown command exits %d", code)
	}
	if code, _, _ := run(srv, "tenants", "create", "a", "b"); code != 2 {
		t.Errorf("too many values exit %d", code)
	}
}

func TestEveryAliasNamesAnOperationOfTheTable(t *testing.T) {
	ids := map[string]bool{}
	for _, o := range server.Operations() {
		ids[o.ID] = true
	}
	seen := map[string]bool{}
	for _, a := range admin.Aliases {
		if !ids[a.Operation] {
			t.Errorf("%s %s: no operation %s", a.Group, a.Verb, a.Operation)
		}
		if key := a.Group + " " + a.Verb; seen[key] {
			t.Errorf("%s is defined twice", key)
		}
		seen[a.Group+" "+a.Verb] = true
	}
}

func TestHelpAndFlagErrorsNeverPrintTheToken(t *testing.T) {
	for _, args := range [][]string{{"-h"}, {"-bogus"}} {
		var out, errOut bytes.Buffer
		admin.Command(&out, &errOut, []string{"VSH_ADMIN_TOKEN=s3cret-operator-token"}).Run(context.Background(), args)
		if all := out.String() + errOut.String(); strings.Contains(all, "s3cret-operator-token") || !strings.Contains(all, "$VSH_ADMIN_TOKEN") {
			t.Fatalf("%v printed: %q", args, all)
		}
	}
}
