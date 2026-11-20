// Package cli is the command line of connector-engine: a set of named subcommands, the core ones and those a build adds.
package cli

import (
	"context"
	"fmt"
	"io"
	"maps"
	"slices"
	"strings"

	"github.com/versatile-sh/connector-engine/cli/admin"
	"github.com/versatile-sh/connector-engine/cli/skills"
	"github.com/versatile-sh/connector-engine/cli/tools"
)

// Command is one cnctl subcommand; it returns the exit code.
type Command func(ctx context.Context, args []string, stdout, stderr io.Writer, environ []string) int

// Core returns the subcommands every cnctl build has: admin over HTTP, skills and tools over MCP.
func Core() map[string]Command {
	return map[string]Command{
		"admin": func(ctx context.Context, args []string, stdout, stderr io.Writer, environ []string) int {
			return admin.Command(stdout, stderr, environ).Run(ctx, args)
		},
		"skills": func(ctx context.Context, args []string, stdout, stderr io.Writer, environ []string) int {
			return exit(stderr, skills.New(Getenv(environ)).Run(ctx, args, stdout, stderr))
		},
		"tools": func(ctx context.Context, args []string, stdout, stderr io.Writer, environ []string) int {
			return exit(stderr, tools.New(Getenv(environ)).Run(ctx, args, stdout, stderr))
		},
	}
}

// Main runs the subcommand args[0] of commands, or prints their names and returns 2.
func Main(ctx context.Context, args []string, stdout, stderr io.Writer, environ []string, commands map[string]Command) int {
	if len(args) > 0 {
		if c, ok := commands[args[0]]; ok {
			return c(ctx, args[1:], stdout, stderr, environ)
		}
	}
	fmt.Fprintf(stderr, "usage: cnctl <command> [arguments]\ncommands: %v\n", slices.Sorted(maps.Keys(commands)))
	return 2
}

// Getenv reads a variable from environ, the KEY=VALUE list a Command receives.
func Getenv(environ []string) func(string) string {
	return func(key string) string {
		for _, kv := range environ {
			if k, v, ok := strings.Cut(kv, "="); ok && k == key {
				return v
			}
		}
		return ""
	}
}

func exit(stderr io.Writer, err error) int {
	if err != nil {
		fmt.Fprintln(stderr, "cnctl:", err)
		return 1
	}
	return 0
}
