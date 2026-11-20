package server_test

import (
	"time"

	"context"
	"encoding/json"
	"net/http"
	"reflect"
	"slices"
	"strings"
	"testing"

	"google.golang.org/grpc/codes"

	"github.com/versatile-sh/connector-engine/port"
	"github.com/versatile-sh/connector-engine/server"
	"github.com/versatile-sh/connector-engine/server/grpcapi"
)

func find(t *testing.T, id string) server.Operation {
	t.Helper()
	for _, o := range server.Operations() {
		if o.ID == id {
			return o
		}
	}
	t.Fatalf("no operation %s", id)
	return server.Operation{}
}

func TestMatchSeparatesActionsFromWildcards(t *testing.T) {
	accept, get := find(t, "acceptFunction"), find(t, "getConnector")
	if p, ok := accept.Match("/v1/connectors/c1/functions/search:accept"); !ok || p["id"] != "c1" || p["name"] != "search" {
		t.Fatalf("accept %v %v", p, ok)
	}
	for _, path := range []string{"/v1/connectors/c1/functions/search", "/v1/connectors/c1/functions/:accept", "/v1/connectors/c1/functions/a:b:accept"} {
		if _, ok := accept.Match(path); ok {
			t.Errorf("accept matched %s", path)
		}
	}
	if _, ok := get.Match("/v1/connectors/c1:enable"); ok {
		t.Error("getConnector swallowed an action")
	}
	if _, ok := find(t, "acceptFunctions").Match("/v1/connectors/c1/functions:accept"); !ok {
		t.Error("acceptFunctions does not match its literal action")
	}
}

func TestEveryRouteIsUnique(t *testing.T) {
	seen := map[string]string{}
	for _, o := range server.Operations() {
		key := o.Method + " " + o.Path
		if prev, dup := seen[key]; dup {
			t.Errorf("%s and %s share %s", prev, o.ID, key)
		}
		seen[key] = o.ID
	}
}

func TestBodilessRequestsCarryOnlyScalarsAndTimes(t *testing.T) {
	for _, o := range server.Operations() {
		for _, name := range append(o.QueryParams(), o.PathParams()...) {
			i := slices.IndexFunc(server.Fields(reflect.TypeOf(o.In()).Elem()), func(f server.Field) bool { return f.Name == name })
			if i < 0 {
				t.Errorf("%s: %s is not a field", o.ID, name)
				continue
			}
			typ := server.Fields(reflect.TypeOf(o.In()).Elem())[i].Type
			switch typ.Kind() {
			case reflect.String, reflect.Int, reflect.Int64, reflect.Bool:
			case reflect.Struct:
				if typ != server.TimeType {
					t.Errorf("%s: %s cannot come from a URL", o.ID, name)
				}
			default:
				t.Errorf("%s: %s cannot come from a URL", o.ID, name)
			}
		}
	}
}

func TestFillParsesQueryTypes(t *testing.T) {
	in := &server.WatchApprovalsRequest{}
	if err := server.Fill(in, map[string]string{"after": "42", "limit": "3"}); err != nil || in.After != 42 || in.Limit != 3 {
		t.Fatalf("%+v %v", in, err)
	}
	if err := server.Fill(in, map[string]string{"after": "x"}); !port.IsCode(err, port.CodeInvalidArgument) {
		t.Fatalf("bad number: %v", err)
	}
	if err := server.Fill(in, map[string]string{"nope": "1"}); !port.IsCode(err, port.CodeInvalidArgument) {
		t.Fatalf("unknown field: %v", err)
	}
	calls := &server.ListCallsRequest{}
	if err := server.Fill(calls, map[string]string{"since": "2026-11-18T09:00:00Z", "limit": "5"}); err != nil || !calls.Since.Equal(time.Date(2026, 11, 18, 9, 0, 0, 0, time.UTC)) || calls.Limit != 5 {
		t.Fatalf("%+v %v", calls, err)
	}
	if err := server.Fill(calls, map[string]string{"since": "yesterday"}); !port.IsCode(err, port.CodeInvalidArgument) {
		t.Fatalf("bad time: %v", err)
	}
}

func TestBearer(t *testing.T) {
	for in, want := range map[string]string{"Bearer abc": "abc", "bearer  abc": "abc", "Basic abc": "", "abc": "", "": ""} {
		if got := server.Bearer(in); got != want {
			t.Errorf("%q: %q, want %q", in, got, want)
		}
	}
}

func TestProblemNeverLeaksInternalDetail(t *testing.T) {
	p := server.ProblemOf(&port.Error{Code: port.CodeInternal, Detail: "dial tcp 10.0.0.1: refused"})
	if p.Status != 500 || p.Detail != "internal error" {
		t.Fatalf("%+v", p)
	}
	p = server.ProblemOf(&port.Error{Code: port.CodeAgentPrincipalRefused, Detail: "no"})
	var doc map[string]any
	data, _ := json.Marshal(p)
	if err := json.Unmarshal(data, &doc); err != nil || p.Status != http.StatusForbidden || doc["code"] != "agent_principal_refused" {
		t.Fatalf("%s %v", data, err)
	}
	if server.ProblemOf(context.Canceled).Status != 500 {
		t.Error("a foreign error is internal")
	}
}

func TestHTTPAndGRPCMapTheSameCodes(t *testing.T) {
	for code := range server.Statuses {
		if _, ok := grpcapi.Codes[code]; !ok {
			t.Errorf("%s has an HTTP status and no gRPC code", code)
		}
	}
	for code, c := range grpcapi.Codes {
		if _, ok := server.Statuses[code]; !ok || c == codes.OK {
			t.Errorf("%s has a gRPC code and no HTTP status", code)
		}
	}
}

func TestNeverOfferedOperationsAreTheDecisionOnes(t *testing.T) {
	var never []string
	for _, o := range server.Operations() {
		if o.Never {
			never = append(never, o.ID)
		}
	}
	slices.Sort(never)
	if !slices.Equal(never, []string{"cancelApproval", "clearPause", "decideApproval"}) {
		t.Fatalf("never offered: %v", never)
	}
}

func typeOf(v any) reflect.Type { return reflect.TypeOf(v).Elem() }

func contains(data []byte, s string) bool { return strings.Contains(string(data), s) }

func replace(s, old, new string) string { return strings.ReplaceAll(s, old, new) }
