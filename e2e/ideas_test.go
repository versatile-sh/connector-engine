package e2e

import (
	"encoding/json"
	"regexp"
	"slices"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/agents-control/connector-engine/port"
)

func init() {
	scenarios = append(scenarios,
		scenario{"idea_files_listing_import_lands_unreviewed", ideaFilesImport},
		scenario{"idea_grpc_kind_call_through_reflection_import", ideaGRPC},
		scenario{"idea_openapi_import_lands_unreviewed_and_calls_rest", ideaOpenAPI},
		scenario{"idea_overlay_accept_and_restore_change_the_served_description", ideaOverlay},
		scenario{"idea_projection_trims_and_result_filter_masks_cpf", ideaProjectionAndFilter},
	)
}

var unsafeTarget = regexp.MustCompile(`[^a-z0-9._-]+`)

func grpcListingName(target string) string {
	return "grpc.reflection/" + unsafeTarget.ReplaceAllString(strings.ToLower(target), "-")
}

func (k *kit) listing(t *testing.T, name string) port.ListingVersion {
	t.Helper()
	var list items[port.ListingVersion]
	k.admin(t).call(t, &list, "listListings", map[string]any{"workspace": workspace})
	for _, lv := range list.Items {
		if lv.Ref.Listing.Name == name {
			return lv
		}
	}
	t.Fatalf("%s is not listed", name)
	return port.ListingVersion{}
}

func (k *kit) landsUnreviewed(t *testing.T, name, source string, kind port.Kind) {
	t.Helper()
	if slices.Contains(k.visible(t), "/"+name) {
		t.Fatalf("%s is visible before the tenant enabled it", name)
	}
	k.catalogRule(t, "/"+name, port.CeilingFull)
	lv := k.listing(t, name)
	if lv.Manifest.TrustLevel != port.TrustUnreviewed || lv.Manifest.Kind != kind || lv.Manifest.Source == nil || lv.Manifest.Source.Registry != source {
		t.Fatalf("%s landed as trust=%s kind=%s source=%+v", name, lv.Manifest.TrustLevel, lv.Manifest.Kind, lv.Manifest.Source)
	}
	k.install(t, name, "g")
	for fname, f := range k.functions(t) {
		if f.State != port.FunctionQuarantined {
			t.Fatalf("%s: function %s is %s after the install of an unreviewed listing", name, fname, f.State)
		}
	}
	k.admin(t).call(t, nil, "setConnectorLevel", map[string]any{"level": port.LevelCustom}, string(k.conn))
	k.grant(t, port.ScopeWorkspace, workspace, "*", port.ModeAllow)
	k.bind(t, "x")
	k.grant(t, port.ScopeAgent, "x", "*", port.ModeAllow)
	if got := k.agent(t, "x", "s1").names(t); len(got) != 0 {
		t.Fatalf("%s is invisible to the agent until the functions are accepted: %v", name, got)
	}
	k.acceptAll(t)
}

func ideaFilesImport(t *testing.T, w *world) {
	writeJSON(t, w.cfg.filesDir+"/late.json", remoteListing("acme.dev/late", "1.0.0", w.mcp.url()))
	var op port.Operation
	w.operator().call(t, &op, "syncSource", nil, "files")
	if op.Counts["imported"] != 1 {
		t.Fatalf("one new file imports one listing: %+v", op.Counts)
	}
	w.operator().call(t, &op, "syncSource", nil, "files")
	if op.Counts["imported"] != 0 {
		t.Fatalf("a second sync imports nothing: %+v", op.Counts)
	}
	k := w.newKit(t, "tfiles")
	k.landsUnreviewed(t, "acme.dev/late", "files", port.KindMCPRemote)
	if got := k.agent(t, "x", "s1").names(t); !slices.Contains(got, "g__search") {
		t.Fatalf("after the accept the imported functions are listed: %v", got)
	}
}

func ideaGRPC(t *testing.T, w *world) {
	name := grpcListingName(w.grpc.Target)
	k := w.newKit(t, "tgrpc")
	k.landsUnreviewed(t, name, "grpc-reflection", port.KindGRPCDeclarative)
	k.classify(t, "Greeter_SayHello", port.ClassRead)
	x := k.agent(t, "x", "s1")
	if got := x.names(t); !slices.Contains(got, "g__Greeter_SayHello") {
		t.Fatalf("after the accept the gRPC method is listed: %v", got)
	}
	before := w.grpc.Calls()
	res, err := x.call("g__Greeter_SayHello", map[string]any{"name": "ana", "count": "41"})
	if err != nil || res.IsError {
		t.Fatalf("grpc call: %v %v", res, err)
	}
	if got := structured(t, res); got["message"] != "hello ana" || got["total"] != "41" {
		t.Fatalf("grpc result: %v", got)
	}
	if n := w.grpc.Calls() - before; n != 1 {
		t.Fatalf("the gRPC server saw %d calls", n)
	}
	failing, err := x.call("g__Greeter_SayHello", map[string]any{"name": "fail"})
	if err != nil || !failing.IsError || !strings.Contains(textOf(failing), "NotFound") {
		t.Fatalf("a gRPC status comes back as a tool error: %v %v", failing, err)
	}
}

func ideaOpenAPI(t *testing.T, w *world) {
	k := w.newKit(t, "trest")
	k.landsUnreviewed(t, "1.0.0.127/pet-store", "openapi", port.KindRESTDeclarative)
	k.classify(t, "listPets", port.ClassRead)
	k.classify(t, "createPet", port.ClassWrite)
	k.classify(t, "deletePet", port.ClassDestructive)
	x := k.agent(t, "x", "s1")
	if got := x.names(t); !slices.Equal(got, append(slices.Clone([]string{"g__createPet", "g__deletePet", "g__listPets"}), "approval_status", "approval_cancel")) {
		t.Fatalf("the REST functions: %v", got)
	}
	res, err := x.call("g__listPets", map[string]any{})
	if err != nil || res.IsError || !strings.Contains(textOf(res), "Rex") || w.rest.count("GET", "/pets") != 1 {
		t.Fatalf("listPets: %v %v", res, err)
	}
	if res, err := x.call("g__createPet", map[string]any{"name": "Tom"}); err != nil || res.IsError || w.rest.count("POST", "/pets") != 1 {
		t.Fatalf("createPet: %v %v", res, err)
	}
	held, err := x.call("g__deletePet", map[string]any{"petId": "p9"})
	if err != nil {
		t.Fatal(err)
	}
	id := isPending(t, held)
	if w.rest.count("DELETE", "/pets/p9") != 0 {
		t.Fatal("a destructive REST function ran before the approval")
	}
	k.decide(t, id, port.DecisionApprove)
	if done := x.answered(t, id); done.IsError || w.rest.count("DELETE", "/pets/p9") != 1 {
		t.Fatalf("the approved delete: %v", done)
	}
}

func ideaOverlay(t *testing.T, w *world) {
	k := w.newKit(t, "toverlay")
	k.issuesConnector(t, port.CeilingFull)
	k.grant(t, port.ScopeWorkspace, workspace, "*", port.ModeAllow)
	k.bind(t, "x")
	k.grant(t, port.ScopeAgent, "x", "fn:search", port.ModeAllow)
	x := k.agent(t, "x", "s1")
	served := func() string { return x.tools(t)["s__search"].Description }
	const original = "Searches the tracker"
	if served() != original {
		t.Fatalf("the upstream description is served first: %q", served())
	}
	hash := k.hashes["search"]
	propose := func(text string) port.Overlay {
		var o port.Overlay
		k.admin(t).call(t, &o, "proposeOverlay", map[string]any{"connector": string(k.conn), "function": "search", "description": text, "upstream_hash": hash})
		return o
	}
	accept := func(op string, version int) port.Overlay {
		var o port.Overlay
		k.admin(t).call(t, &o, op, map[string]any{"connector": string(k.conn), "function": "search", "upstream_seen": hash}, itoa(version))
		return o
	}
	first := propose("Finds issues in the team tracker")
	if served() != original {
		t.Fatalf("a proposed overlay is not served: %q", served())
	}
	accept("acceptOverlay", first.Version)
	if served() != "Finds issues in the team tracker" {
		t.Fatalf("the accepted overlay is served: %q", served())
	}
	second := propose("Searches issues by words, newest first")
	accept("acceptOverlay", second.Version)
	if served() != "Searches issues by words, newest first" {
		t.Fatalf("the second overlay is served: %q", served())
	}
	accept("restoreOverlay", first.Version)
	if served() != "Finds issues in the team tracker" {
		t.Fatalf("a restored overlay is served again: %q", served())
	}
	k.admin(t).call(t, nil, "clearOverlay", map[string]any{"connector": string(k.conn), "function": "search"})
	if served() != original {
		t.Fatalf("clearing goes back to the upstream text: %q", served())
	}
}

func ideaProjectionAndFilter(t *testing.T, w *world) {
	k := w.newKit(t, "tproj")
	k.issuesConnector(t, port.CeilingFull)
	k.grant(t, port.ScopeWorkspace, workspace, "*", port.ModeAllow)
	k.bind(t, "x")
	k.grant(t, port.ScopeAgent, "x", "fn:customer", port.ModeAllow)
	x := k.agent(t, "x", "s1")

	full, err := x.call("s__customer", map[string]any{})
	if err != nil || full.IsError {
		t.Fatalf("customer: %v %v", full, err)
	}
	raw, _ := json.Marshal(full)
	if strings.Contains(string(raw), customerCPF) {
		t.Fatalf("the result filter lets the CPF through: %s", raw)
	}
	got := structured(t, full)
	if got["cpf"] == "" || got["cpf"] == nil || got["note"] != "vip" {
		t.Fatalf("the filter masks the CPF and leaves the rest: %v", got)
	}
	k.admin(t).call(t, nil, "setProjection", map[string]any{"on": true}, string(k.conn), "customer")
	trimmed, err := x.callFields("s__customer", map[string]any{}, "/name")
	if err != nil || trimmed.IsError {
		t.Fatalf("projected: %v %v", trimmed, err)
	}
	if got := structured(t, trimmed); len(got) != 1 || got["name"] != "Ana" {
		t.Fatalf("the projection keeps the named path only: %v", got)
	}
	raw, _ = json.Marshal(trimmed)
	if strings.Contains(string(raw), "vip") || strings.Contains(string(raw), customerCPF) {
		t.Fatalf("the trimmed result leaks: %s", raw)
	}
	bad, err := x.callFields("s__customer", map[string]any{}, "/missing")
	if err != nil || !bad.IsError || structured(t, bad)["code"] != string(port.CodeInvalidPath) {
		t.Fatalf("a path outside the output schema is a tool error: %v %v", bad, err)
	}
	var _ mcp.Tool
}
