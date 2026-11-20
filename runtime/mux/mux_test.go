package mux_test

import (
	"context"
	"testing"

	"github.com/versatile-sh/connector-engine/port"
	rtmemory "github.com/versatile-sh/connector-engine/runtime/memory"
	"github.com/versatile-sh/connector-engine/runtime/mux"
)

func TestKindsGoToTheirRuntimeAndHandlesFollow(t *testing.T) {
	ctx := context.Background()
	a, b := rtmemory.New(), rtmemory.New()
	a.Script("c1", rtmemory.Script{Definitions: nil})
	m, err := mux.New(a, b)
	if err != nil {
		t.Fatal(err)
	}
	if kinds := m.Describe().Kinds; len(kinds) != 4 {
		t.Fatalf("kinds %v", kinds)
	}
	h, err := m.Start(ctx, port.Connector{ID: "c1"}, port.Manifest{Kind: port.KindMCPRemote}, port.SecretsRef{})
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := m.ListFunctions(ctx, h); err != nil {
		t.Fatal(err)
	}
	if err := m.Stop(ctx, h); err != nil {
		t.Fatal(err)
	}
	if _, _, err := m.ListFunctions(ctx, h); !port.IsCode(err, port.CodeNotFound) {
		t.Fatalf("a stopped handle answered: %v", err)
	}
	if _, err := m.Start(ctx, port.Connector{}, port.Manifest{Kind: port.KindPiece}, port.SecretsRef{}); !port.IsCode(err, port.CodeInvalidManifest) {
		t.Fatalf("a kind nobody serves started: %v", err)
	}
	if _, err := mux.New(); err == nil {
		t.Fatal("an empty mux was built")
	}
}
