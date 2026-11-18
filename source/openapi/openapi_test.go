package openapi_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/agents-control/connector-engine/core/listing"
	"github.com/agents-control/connector-engine/internal/egress"
	"github.com/agents-control/connector-engine/internal/sourcetest"
	"github.com/agents-control/connector-engine/port"
	"github.com/agents-control/connector-engine/source/openapi"
)

func TestImportsADirectoryAsRESTListings(t *testing.T) {
	got := sourcetest.Import(t, openapi.New(openapi.Config{Dir: "testdata"}))
	m := got["com.example.petstore.eu/pet-store"]
	if m.Kind != port.KindRESTDeclarative || m.Version != "1.4.0" || m.Publisher.Name != "Pet Store Inc" ||
		m.REST.BaseURL != "https://eu.petstore.example.com:8443/v1" || len(m.Egress.AllowHosts) != 1 || m.Egress.AllowHosts[0] != "eu.petstore.example.com:8443" {
		t.Fatalf("manifest: %+v", m)
	}
	fns := map[string]port.RESTFunction{}
	for _, f := range m.REST.Functions {
		fns[f.Name] = f
	}
	if len(fns) != 4 || fns["upload"].Name != "" {
		t.Fatalf("functions %v: the multipart operation cannot be executed and is left out", fns)
	}

	list := fns["listPets"]
	if list.Method != "GET" || list.Path != "/pets" || list.Description != "List pets" {
		t.Fatalf("listPets: %+v", list)
	}
	if props := properties(t, list.InputSchema); props["limit"]["x-in"] != "query" || props["X-Trace"] != nil {
		t.Fatalf("listPets input: %s", list.InputSchema)
	}
	if !strings.Contains(string(list.OutputSchema), `"pets"`) {
		t.Fatalf("listPets output: %s", list.OutputSchema)
	}

	create := fns["createPet"]
	var in struct {
		Required []string `json:"required"`
	}
	json.Unmarshal(create.InputSchema, &in)
	props := properties(t, create.InputSchema)
	if create.Method != "POST" || props["dry_run"]["x-in"] != "query" || props["name"] == nil || props["name"]["x-in"] != nil || len(in.Required) != 1 || in.Required[0] != "name" {
		t.Fatalf("createPet input: %s", create.InputSchema)
	}
	if strings.Count(string(create.OutputSchema), `"friend"`) < 1 || strings.Contains(string(create.OutputSchema), "$ref") {
		t.Fatalf("createPet output must have its references resolved: %s", create.OutputSchema)
	}

	read := fns["get_pets_petId"]
	if read.Method != "GET" || read.Path != "/pets/{petId}" || !strings.Contains(string(read.InputSchema), `"required":["petId"]`) {
		t.Fatalf("a path-level parameter and a missing operationId: %+v", read)
	}
	if fns["deletePet"].Method != "DELETE" || fns["deletePet"].OutputSchema != nil {
		t.Fatalf("deletePet: %+v", fns["deletePet"])
	}
}

func TestImportsFromAnAddress(t *testing.T) {
	doc, err := os.ReadFile("testdata/petstore.yaml")
	if err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/spec.yaml" {
			http.NotFound(w, r)
			return
		}
		w.Write(doc)
	}))
	defer srv.Close()
	got := sourcetest.Import(t, openapi.New(openapi.Config{Guard: egress.Guard{AllowPrivate: true}, URLs: []string{srv.URL + "/spec.yaml"}}))
	if len(got) != 1 {
		t.Fatalf("imported %d", len(got))
	}
	if _, err := openapi.New(openapi.Config{Guard: egress.Guard{AllowPrivate: true}, URLs: []string{srv.URL + "/missing"}}).List(t.Context(), ""); !port.IsCode(err, port.CodeNotFound) {
		t.Fatalf("missing document: %v", err)
	}
}

func TestGETIsASuggestedRead(t *testing.T) {
	got := sourcetest.Import(t, openapi.New(openapi.Config{Dir: "testdata"}))
	for _, f := range got["com.example.petstore.eu/pet-store"].REST.Functions {
		class, source, suggested := listing.Class(listing.ClassInput{HTTPMethod: f.Method})
		want := map[string]port.Class{"GET": port.ClassRead, "POST": port.ClassWrite, "DELETE": port.ClassDestructive}[f.Method]
		if class != port.ClassNone || source != port.ClassSourceDefault || suggested != want {
			t.Fatalf("%s %s: class %q source %q suggested %q", f.Method, f.Name, class, source, suggested)
		}
	}
}

func TestRejectsWhatIsNotOpenAPI3(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(dir+"/old.yaml", []byte("swagger: '2.0'\ninfo: {title: x, version: '1'}\n"), 0o600)
	if _, err := openapi.New(openapi.Config{Dir: dir}).List(t.Context(), ""); !port.IsCode(err, port.CodeInvalidManifest) {
		t.Fatalf("got %v", err)
	}
}

func properties(t *testing.T, schema json.RawMessage) map[string]map[string]any {
	t.Helper()
	var s struct {
		Properties map[string]map[string]any `json:"properties"`
	}
	if err := json.Unmarshal(schema, &s); err != nil {
		t.Fatal(err)
	}
	return s.Properties
}
