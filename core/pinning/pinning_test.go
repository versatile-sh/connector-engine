package pinning_test

import (
	"testing"
	"time"

	"github.com/versatile-sh/connector-engine/core/pinning"
	"github.com/versatile-sh/connector-engine/port"
)

func TestCanonical(t *testing.T) {
	for in, want := range map[string]string{
		`{"b":1,"a":[true,null,"x"]}`: `{"a":[true,null,"x"],"b":1}`,
		`{"numbers":[333333333.33333329,1E30,4.50,2e-3,0.000000000000000000000000001]}`: `{"numbers":[333333333.3333333,1e+30,4.5,0.002,1e-27]}`,
		`{"string":"\u20ac$\u000F\u000aA'\u0042\u0022\u005c\\\"\/"}`:                    "{\"string\":\"\u20ac$\\u000f\\nA'B\\\"\\\\\\\\\\\"/\"}",
		`{"\u20ac":1,"\r":2,"1":3,"\ud83d\ude00":4,"\u0080":5,"\u00f6":6}`:              "{\"\\r\":2,\"1\":3,\"\u0080\":5,\"\u00f6\":6,\"\u20ac\":1,\"\U0001F600\":4}",
		`[-0, 1e21, 1e-7, 100]`: `[0,1e+21,1e-7,100]`,
	} {
		got, err := pinning.Canonical([]byte(in))
		if err != nil || string(got) != want {
			t.Errorf("RFC 8785 of %s: %s (%v), want %s", in, got, err, want)
		}
	}
	if _, err := pinning.Canonical([]byte(`{"a":1} {}`)); err == nil {
		t.Error("trailing data accepted")
	}
}

func TestDefinitionHash(t *testing.T) {
	h := func(raw string) port.Hash {
		t.Helper()
		v, err := pinning.DefinitionHash("f", []byte(raw))
		if err != nil {
			t.Fatal(err)
		}
		return v
	}
	base := h(`{"name":"f","description":"d","inputSchema":{"type":"object"}}`)
	if h(`{"inputSchema":{"type":"object"},"description":"d","name":"f","icons":[{"src":"x"}],"_meta":{"other":1}}`) != base {
		t.Fatal("icons, other _meta keys and key order are not hashed")
	}
	if h(`{"name":"f","description":"d","inputSchema":{"type":"object"},"annotations":{"readOnlyHint":false}}`) == base {
		t.Fatal("an explicit readOnlyHint false differs from an absent one")
	}
	if h(`{"name":"f","description":"d","inputSchema":{"type":"object"},"_meta":{"ui":{"resourceUri":"ui://a"}}}`) == base {
		t.Fatal("_meta ui.resourceUri is hashed")
	}
	fields, err := pinning.ChangedFields("f", []byte(`{"name":"f","description":"d"}`), []byte(`{"name":"f","description":"e","title":"t"}`))
	if err != nil || len(fields) != 2 || fields[0] != "title" || fields[1] != "description" {
		t.Fatalf("changed fields %v %v", fields, err)
	}
	if pinning.OverlayHash("a", "b") == pinning.OverlayHash("a", "c") || pinning.InstructionsHash("x") == pinning.InstructionsHash("y") {
		t.Fatal("hashes distinguish their texts")
	}
	a := port.Approval{Tenant: "t", Agent: "a", Connector: "c", Function: "f", DefinitionHash: "h"}
	x, _ := pinning.CallHash(a, []byte(`{"b":1,"a":2}`))
	y, _ := pinning.CallHash(a, []byte(`{ "a":2, "b":1 }`))
	z, _ := pinning.CallHash(a, []byte(`{"a":2,"b":2}`))
	if x != y || x == z {
		t.Fatal("call_hash is canonical over the arguments and binds them exactly")
	}
}

func TestCallHashGolden(t *testing.T) {
	a := port.Approval{Tenant: "t", Agent: "a", Connector: "c", Function: "f", DefinitionHash: "h"}
	got, err := pinning.CallHash(a, []byte(`{"b":[1,{"y":"é","x":null}],"a":{"k":1.5,"j":"v"}}`))
	if err != nil || got != "1e5ebbd2001716ff5e945e3ee01f688e19e22aa54f75e1ad91dd5c2155320071" {
		t.Fatalf("call_hash of a fixed call changed: %s %v", got, err)
	}
}

func TestReconcile(t *testing.T) {
	pinned := []port.Function{
		{Name: "a", DefinitionHash: "a1", State: port.FunctionActive},
		{Name: "b", DefinitionHash: "b1", State: port.FunctionActive},
		{Name: "c", DefinitionHash: "c1", State: port.FunctionDisabled},
		{Name: "d", DefinitionHash: "d1", State: port.FunctionRemoved, PreviousState: port.FunctionActive},
		{Name: "e", DefinitionHash: "e1", State: port.FunctionRemoved, PreviousState: port.FunctionActive},
	}
	captured := []port.Function{
		{Name: "a", DefinitionHash: "a1"}, {Name: "b", DefinitionHash: "b2"}, {Name: "d", DefinitionHash: "d1"},
		{Name: "e", DefinitionHash: "e2"}, {Name: "n", DefinitionHash: "n1", Class: port.ClassRead},
	}
	out, changes := pinning.Reconcile(pinned, captured)
	state := map[string]port.FunctionState{}
	for _, f := range out {
		state[f.Name] = f.State
	}
	want := map[string]port.FunctionState{"a": port.FunctionActive, "b": port.FunctionQuarantined, "c": port.FunctionRemoved, "d": port.FunctionActive, "e": port.FunctionQuarantined, "n": port.FunctionQuarantined}
	for k, v := range want {
		if state[k] != v {
			t.Errorf("%s is %s, want %s", k, state[k], v)
		}
	}
	kinds := map[string]pinning.ChangeKind{}
	for _, c := range changes {
		kinds[c.Function] = c.Kind
		if c.Kind == pinning.ChangeChanged && (!c.Effects.DropSessionRows || c.Effects.Cancel != port.ReasonQuarantined) {
			t.Errorf("hash changed: session grants deleted, pending approvals cancelled: %+v", c)
		}
	}
	if kinds["b"] != pinning.ChangeChanged || kinds["c"] != pinning.ChangeRemoved || kinds["d"] != pinning.ChangeReturned || kinds["n"] != pinning.ChangeNew || kinds["e"] != pinning.ChangeChanged || len(changes) != 5 {
		t.Fatalf("changes %+v", changes)
	}
}

func TestAcceptAndReclassify(t *testing.T) {
	f := pinning.New(port.Function{Name: "f", DefinitionHash: "h2", Class: port.ClassRead})
	if _, err := pinning.Accept(f, "h1", "admin", time.Time{}); !port.IsCode(err, port.CodeDefinitionChanged) {
		t.Fatalf("accepting names the hash seen: %v", err)
	}
	f, err := pinning.Accept(f, "h2", "admin", time.Time{})
	if err != nil || f.State != port.FunctionActive {
		t.Fatalf("accept: %v %s", err, f.State)
	}
	if _, eff := pinning.Reclassify(f, port.ClassWrite, port.ClassSourceAdmin); !eff.DropSessionRows || eff.Cancel != port.CodeClassChanged {
		t.Fatal("G7: a raised class drops session rows and cancels pending approvals")
	}
	if _, eff := pinning.Reclassify(port.Function{Class: port.ClassDestructive}, port.ClassRead, port.ClassSourceAdmin); eff.DropSessionRows {
		t.Fatal("G7: lowering a class drops nothing")
	}
	if _, eff := pinning.Reclassify(port.Function{}, port.ClassDestructive, port.ClassSourceAdmin); eff.DropSessionRows {
		t.Fatal("unclassified is already destructive")
	}
	if pinning.Captured(port.Function{}, port.TrustSigned).State != port.FunctionQuarantined || pinning.Captured(port.Function{}, port.TrustReviewed).State != port.FunctionActive {
		t.Fatal("captured at install of a reviewed listing is active, else quarantined")
	}
	review := &port.Review{DefinitionHashes: []port.Hash{"h2"}}
	read := port.Function{DefinitionHash: "h2", Class: port.ClassRead, ClassSource: port.ClassSourceReviewed}
	if pinning.AutoAccept(read, review, "none") || !pinning.AutoAccept(read, review, "read") {
		t.Fatal("auto-accept only with pinning.autoAcceptReviewed = read")
	}
}

func TestAnItemPinsTheKeysOfItsKind(t *testing.T) {
	h := func(raw string) port.Hash {
		v, err := pinning.DefinitionHash(port.InstructionsItem, []byte(raw))
		if err != nil {
			t.Fatal(err)
		}
		return v
	}
	if h(`{"instructions":"a"}`) == h(`{"instructions":"b"}`) || h(`{"instructions":"a"}`) != h(`{"instructions":"a","description":"x"}`) {
		t.Fatal("instructions pin their text and only it")
	}
	fields, err := pinning.ChangedFields(port.InstructionsItem, []byte(`{"instructions":"a"}`), []byte(`{"instructions":"b"}`))
	if err != nil || len(fields) != 1 || fields[0] != "instructions" {
		t.Fatalf("changed fields %v %v", fields, err)
	}
}
