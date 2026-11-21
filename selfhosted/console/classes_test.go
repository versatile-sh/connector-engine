package console_test

import (
	"context"
	"slices"
	"testing"
	"time"

	"github.com/versatile-sh/connector-engine/port"
	"github.com/versatile-sh/connector-engine/selfhosted/console"
)

func (r *rig) function(name string) console.FunctionView {
	r.t.Helper()
	var list struct{ Connectors []console.ConnectorSummary }
	r.b.ok("GET", "/api/connectors", nil, &list)
	for _, cv := range list.Connectors {
		for _, f := range cv.Functions {
			if cv.ID == r.conn.ID && f.Name == name {
				return f
			}
		}
	}
	r.t.Fatalf("function %s not listed", name)
	return console.FunctionView{}
}

func (r *rig) engineClass(name string, class port.Class) {
	r.t.Helper()
	if err := r.eng.Call(console.WithActor(context.Background(), "owner"), "PUT", "/v1/connectors/"+r.conn.ID+"/functions/"+name+"/class", map[string]any{"class": class}, nil); err != nil {
		r.t.Fatal(err)
	}
}

func (r *rig) heldAtOnce(function string) {
	r.t.Helper()
	before := r.up.count(function)
	pending := r.call(r.connect("", nil), function, map[string]any{"at": time.Now().UnixNano()})
	r.approveFirstPending()
	ran(r.t, <-pending, function)
	if r.up.count(function) != before+1 {
		r.t.Fatalf("%s ran %d times", function, r.up.count(function)-before)
	}
}

func TestEnablingADeclaredDestructiveFunctionKeepsTheCap(t *testing.T) {
	r := newRig(t)
	r.b.ok("PUT", "/api/connectors/"+r.conn.ID+"/functions/purge", map[string]string{"access": "on"}, nil)
	if f := r.function("purge"); f.Class != port.ClassDestructive || f.Access != console.AccessOn {
		t.Fatalf("purge after on: %+v", f)
	}
	r.heldAtOnce("purge")
}

func TestEnablingAnUnreviewedFunctionClassesItByItsHints(t *testing.T) {
	r := newRig(t)
	for _, name := range []string{"create", "search"} {
		r.b.ok("PUT", "/api/connectors/"+r.conn.ID+"/functions/"+name, map[string]string{"access": "on"}, nil)
	}
	if f := r.function("create"); f.Class != port.ClassWrite {
		t.Fatalf("create after on: %+v", f)
	}
	if f := r.function("search"); f.Class != port.ClassRead {
		t.Fatalf("search after on: %+v", f)
	}
	cs := r.connect("", nil)
	for _, name := range []string{"create", "search"} {
		before := r.up.count(name)
		ran(t, <-r.call(cs, name, map[string]any{}), name)
		if r.up.count(name) != before+1 {
			t.Fatalf("%s on did not run at once", name)
		}
	}
}

func TestACappedFunctionIsNotOfferedAlways(t *testing.T) {
	r := newRig(t)
	pending := r.call(r.connect("", nil), "purge", map[string]any{})
	var held struct {
		Approvals []struct {
			ID      string
			Choices []string
		}
	}
	r.eventually("a held purge", func() bool {
		r.b.ok("GET", "/api/approvals", nil, &held)
		return len(held.Approvals) > 0
	})
	if a := held.Approvals[0]; slices.Contains(a.Choices, "always") || !slices.Contains(a.Choices, "session") {
		t.Fatalf("choices for a destructive function: %v", a.Choices)
	}
	r.approveFirstPending()
	ran(t, <-pending, "purge")
}

func TestUpgradeRepairRaisesConsoleLoweredClasses(t *testing.T) {
	r := newRig(t)
	ctx := console.WithActor(context.Background(), "console")
	r.engineClass("purge", port.ClassWrite)
	cs := r.connect("", nil)
	pending := r.call(cs, "purge", map[string]any{"n": 1})
	var held struct{ Approvals []struct{ ID string } }
	r.eventually("a held purge", func() bool {
		r.b.ok("GET", "/api/approvals", nil, &held)
		return len(held.Approvals) > 0
	})
	r.b.ok("POST", "/api/approvals/"+held.Approvals[0].ID+"/decide", map[string]any{"choice": "session"}, nil)
	ran(t, <-pending, "purge")
	ran(t, <-r.call(cs, "purge", map[string]any{"n": 2}), "purge")

	if err := r.c.RepairClasses(ctx); err != nil {
		t.Fatal(err)
	}
	if f := r.function("purge"); f.Class != port.ClassDestructive {
		t.Fatalf("purge after the repair: %+v", f)
	}
	before := r.up.count("purge")
	pending = r.call(cs, "purge", map[string]any{"n": 3})
	r.approveFirstPending()
	ran(t, <-pending, "purge")
	if r.up.count("purge") != before+1 {
		t.Fatal("the session answer outlived the raise")
	}
	raised := func() int {
		n := 0
		for _, e := range r.c.State.Audit(100) {
			if e.Action == "function.class_raised" && e.Detail["function"] == "purge" {
				n++
			}
		}
		return n
	}
	if raised() != 1 {
		t.Fatalf("raises recorded: %d", raised())
	}

	r.engineClass("purge", port.ClassWrite)
	if err := r.c.RepairClasses(ctx); err != nil {
		t.Fatal(err)
	}
	if f := r.function("purge"); f.Class != port.ClassWrite || raised() != 1 {
		t.Fatalf("a second repair changed a deliberate lowering: %+v, %d raises", f, raised())
	}
}

func TestAReviewedClassIsTheFloorForTheRepairAndForOn(t *testing.T) {
	r := newRig(t)
	ctx := console.WithActor(context.Background(), "console")
	var fns struct{ Items []port.Function }
	if err := r.eng.Read(ctx, "GET", "/v1/connectors/"+r.conn.ID+"/functions", nil, &fns); err != nil {
		t.Fatal(err)
	}
	hash := map[string]port.Hash{}
	for _, f := range fns.Items {
		hash[f.Name] = f.DefinitionHash
	}
	review := map[string]any{"owner": "local", "name": r.conn.Listing, "version": r.conn.Version, "functions": []map[string]any{
		{"name": "search", "hash": hash["search"], "class": port.ClassWrite}, {"name": "create", "hash": hash["create"], "class": port.ClassDestructive}}}
	if err := r.eng.Call(ctx, "POST", "/v1/listings:review", review, nil); err != nil {
		t.Fatal(err)
	}
	r.engineClass("create", port.ClassRead)
	if err := r.c.RepairClasses(ctx); err != nil {
		t.Fatal(err)
	}
	if f := r.function("create"); f.Class != port.ClassDestructive || f.Kind != port.ClassDestructive {
		t.Fatalf("create after the repair: %+v", f)
	}
	if f := r.function("search"); f.Kind != port.ClassWrite {
		t.Fatalf("search reviewed as a change reads %+v", f)
	}
	r.b.ok("PUT", "/api/connectors/"+r.conn.ID+"/functions/search", map[string]string{"access": "on"}, nil)
	if f := r.function("search"); f.Class != port.ClassWrite {
		t.Fatalf("search reviewed as a change, turned on: %+v", f)
	}
}
