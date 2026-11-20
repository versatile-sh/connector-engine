// Command gen writes api/connectorengine/v1/connectorengine.proto, its Go bindings and api/openapi.yaml from the operation table; run it through go generate.
package main

import (
	"log"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/versatile-sh/connector-engine/server"
)

const module = "github.com/versatile-sh/connector-engine"

func main() {
	root := os.Args[1]
	proto, err := server.Proto()
	check(err)
	spec, err := server.OpenAPI()
	check(err)
	check(os.MkdirAll(filepath.Join(root, "api/connectorengine/v1"), 0o755))
	check(os.WriteFile(filepath.Join(root, "api/connectorengine/v1/connectorengine.proto"), proto, 0o644))
	check(os.WriteFile(filepath.Join(root, "api/openapi.yaml"), spec, 0o644))
	cmd := exec.Command("protoc", "-I", "api", "--go_out=.", "--go_opt=module="+module, "--go-grpc_out=.", "--go-grpc_opt=module="+module, "api/connectorengine/v1/connectorengine.proto")
	cmd.Dir, cmd.Stderr = root, os.Stderr
	check(cmd.Run())
}

func check(err error) {
	if err != nil {
		log.Fatal(err)
	}
}
