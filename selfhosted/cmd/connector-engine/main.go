// Command connector-engine-selfhosted is the engine of a person install: the core engine plus the hostkey and sqlite adapters, its unlock command, and the up, stop, status, export, import, reset-password and uninstall commands of an install without Docker.
package main

import (
	"maps"
	"os"

	"github.com/versatile-sh/connector-engine/app"
	"github.com/versatile-sh/connector-engine/selfhosted"
	"github.com/versatile-sh/connector-engine/selfhosted/standalone"
)

var version = "dev"

func main() {
	adapters := app.Core()
	selfhosted.Register(adapters)
	commands := selfhosted.EngineCommands()
	maps.Copy(commands, standalone.Commands(version))
	os.Exit(app.Main(app.Build{Version: version, Adapters: adapters, Commands: commands}, os.Args[1:], os.Environ(), os.Stdin, os.Stdout, os.Stderr))
}
