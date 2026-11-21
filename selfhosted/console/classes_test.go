package console_test

import (
	"context"
	"net/http"
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

func TestEnablingAnUnhintedFunctionLeavesItUnclassified(t *testing.T) {
	r := newRig(t)
	r.b.ok("PUT", "/api/connectors/"+r.conn.ID+"/functions/create", map[string]string{"access": "on"}, nil)
	if f := r.function("create"); f.Class != port.ClassNone || f.Access != console.AccessOn {
		t.Fatalf("create after on: %+v", f)
	}
	r.heldAtOnce("create")
}

func TestAReadOnlyHintIsTrustedOnlyByAnExplicitChoice(t *testing.T) {
	r := newRig(t)
	if f := r.function("search"); f.Class != port.ClassNone || f.Kind != port.ClassRead || f.Access != console.AccessOn {
		t.Fatalf("search proposed on: %+v", f)
	}
	r.heldAtOnce("search")
	if status, _ := r.b.send("PUT", "/api/connectors/"+r.conn.ID+"/read-only", map[string]string{"name": "purge"}, nil); status != http.StatusBadRequest {
		t.Fatalf("a declared destructive function taken as read-only: %d", status)
	}
	if status, _ := r.b.send("PUT", "/api/connectors/"+r.conn.ID+"/read-only", map[string]string{"name": "create"}, nil); status != http.StatusBadRequest {
		t.Fatalf("a function without a read-only hint taken as read-only: %d", status)
	}
	r.b.ok("PUT", "/api/connectors/"+r.conn.ID+"/read-only", map[string]string{"name": "search"}, nil)
	if f := r.function("search"); f.Class != port.ClassRead {
		t.Fatalf("search after the choice: %+v", f)
	}
	before := r.up.count("search")
	ran(t, <-r.call(r.connect("", nil), "search", map[string]any{}), "search")
	if r.up.count("search") != before+1 {
		t.Fatal("a trusted read did not run at once")
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
	ctx := console.WithActor(context.Background(), "owner")
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
