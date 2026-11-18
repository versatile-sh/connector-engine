package memory_test

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/agents-control/connector-engine/port"
	"github.com/agents-control/connector-engine/runtime/memory"
)

func TestScriptedConnector(t *testing.T) {
	ctx := context.Background()
	r := memory.New()
	if _, err := r.Start(ctx, port.Connector{ID: "c1"}, port.Manifest{}, port.SecretsRef{}); !port.IsCode(err, port.CodeNotFound) {
		t.Fatalf("start without a script: %v", err)
	}
	r.Script("c1", memory.Script{
		Definitions: []port.RawDefinition{{Name: "echo", Raw: json.RawMessage(`{"name":"echo"}`)}},
		Handler: func(ctx context.Context, fn string, args json.RawMessage) (port.Result, error) {
			<-ctx.Done()
			return port.Result{Content: args}, ctx.Err()
		},
	})
	h, err := r.Start(ctx, port.Connector{ID: "c1"}, port.Manifest{}, port.SecretsRef{Credential: "cred"})
	if err != nil {
		t.Fatal(err)
	}
	defs, _, err := r.ListFunctions(ctx, h)
	if err != nil || len(defs) != 1 || defs[0].Name != "echo" {
		t.Fatalf("list: %v %v", defs, err)
	}
	r.Script("c1", memory.Script{Definitions: []port.RawDefinition{{Name: "echo2"}}, Handler: func(context.Context, string, json.RawMessage) (port.Result, error) {
		return port.Result{Structured: json.RawMessage(`{"ok":true}`)}, nil
	}})
	if defs, _, _ = r.ListFunctions(ctx, h); defs[0].Name != "echo2" {
		t.Fatal("a replaced script is not served")
	}
	res, err := r.Call(ctx, h, "echo2", json.RawMessage(`{"a":1}`), time.Now().Add(time.Minute))
	if err != nil || string(res.Structured) != `{"ok":true}` {
		t.Fatalf("call: %v %v", res, err)
	}
	calls := r.Calls()
	if len(calls) != 1 || calls[0].Function != "echo2" || string(calls[0].Arguments) != `{"a":1}` || calls[0].Secrets.Credential != "cred" {
		t.Fatalf("calls %+v", calls)
	}
	if err := r.Stop(ctx, h); err != nil {
		t.Fatal(err)
	}
	if _, err := r.Call(ctx, h, "echo2", nil, time.Time{}); !port.IsCode(err, port.CodeNotFound) {
		t.Fatalf("call after stop: %v", err)
	}
}

func TestDeadlineReachesTheHandler(t *testing.T) {
	ctx := context.Background()
	r := memory.New()
	r.Script("c1", memory.Script{Handler: func(ctx context.Context, _ string, _ json.RawMessage) (port.Result, error) {
		<-ctx.Done()
		return port.Result{}, ctx.Err()
	}})
	h, _ := r.Start(ctx, port.Connector{ID: "c1"}, port.Manifest{}, port.SecretsRef{})
	if _, err := r.Call(ctx, h, "f", nil, time.Now().Add(20*time.Millisecond)); err != context.DeadlineExceeded {
		t.Fatalf("got %v", err)
	}
}
