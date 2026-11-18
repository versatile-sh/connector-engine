package server_test

import (
	"os"
	"slices"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"

	"github.com/agents-control/connector-engine/server"
)

func TestGeneratedFilesAreFresh(t *testing.T) {
	for path, build := range map[string]func() ([]byte, error){"../api/connectorengine/v1/connectorengine.proto": server.Proto, "../api/openapi.yaml": server.OpenAPI} {
		want, err := build()
		if err != nil {
			t.Fatal(err)
		}
		got, err := os.ReadFile(path)
		if err != nil || string(got) != string(want) {
			t.Errorf("%s is stale or missing: run go generate ./server", path)
		}
	}
}

func TestOpenAPIListsEveryOperationOnce(t *testing.T) {
	data, err := server.OpenAPI()
	if err != nil {
		t.Fatal(err)
	}
	var spec struct {
		Paths map[string]map[string]struct {
			OperationID string                    `yaml:"operationId"`
			Responses   map[string]map[string]any `yaml:"responses"`
		}
	}
	if err := yaml.Unmarshal(data, &spec); err != nil {
		t.Fatal(err)
	}
	var got, want []string
	for path, methods := range spec.Paths {
		for method, o := range methods {
			got = append(got, method+" "+path+" "+o.OperationID)
			if o.Responses["default"] == nil {
				t.Errorf("%s has no problem response", o.OperationID)
			}
		}
	}
	for _, o := range server.Operations() {
		want = append(want, strings.ToLower(o.Method)+" "+o.Path+" "+o.ID)
	}
	slices.Sort(got)
	slices.Sort(want)
	if !slices.Equal(got, want) {
		t.Fatalf("openapi %v, table %v", got, want)
	}
}
