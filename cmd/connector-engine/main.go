// Command connector-engine boots the engine from connector-engine.yaml and adapters.yaml, checks an adapters file, or writes the first-boot secrets of a compose install.
package main

import (
	"os"

	"github.com/versatile-sh/connector-engine/app"
)

var version = "dev"

func main() {
	os.Exit(app.Main(app.Build{Version: version}, os.Args[1:], os.Environ(), os.Stdin, os.Stdout, os.Stderr))
}
