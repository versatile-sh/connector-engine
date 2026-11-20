// Command connector-engine-selfhosted is the engine of a Docker person install: the core engine plus the hostkey and sqlite adapters and its unlock command.
package main

import (
	"os"

	"github.com/versatile-sh/connector-engine/app"
	"github.com/versatile-sh/connector-engine/selfhosted"
)

var version = "dev"

func main() {
	adapters := app.Core()
	selfhosted.Register(adapters)
	os.Exit(app.Main(app.Build{Version: version, Adapters: adapters, Commands: selfhosted.EngineCommands()}, os.Args[1:], os.Environ(), os.Stdin, os.Stdout, os.Stderr))
}
