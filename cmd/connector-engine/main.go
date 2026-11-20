// Command connector-engine boots the engine from connector-engine.yaml and adapters.yaml, checks an adapters file, or delivers the unlock key of the hostkey adapter.
package main

import (
	"os"

	"github.com/versatile-sh/connector-engine/app"
	"github.com/versatile-sh/connector-engine/selfhosted"
)

var version = "dev"

func main() {
	r := app.Core()
	selfhosted.Register(r)
	os.Exit(app.Main(app.Build{Version: version, Adapters: r, Commands: selfhosted.EngineCommands()}, os.Args[1:], os.Environ(), os.Stdin, os.Stdout, os.Stderr))
}
