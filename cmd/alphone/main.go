// SPDX-License-Identifier: Elastic-2.0

// Command alphone runs the AlphOne CRM server and the commands an operator runs beside it.
package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"slices"
	"syscall"

	"github.com/joho/godotenv"

	"github.com/gopherium/framework/gonsole"

	"github.com/gopherium/alphone/internal/role"
	"github.com/gopherium/alphone/sdk"
)

// oldCommands are the commands the old dispatch still runs ahead of the command line.
var oldCommands = []string{"createadmin", "grantrole", "token"}

// main runs the alphone command line, an old command through the old dispatch.
func main() {
	_ = godotenv.Load()
	if len(os.Args) > 1 && slices.Contains(oldCommands, os.Args[1]) {
		os.Exit(runOld(os.Args[1:]))
	}
	os.Exit(gonsole.Main(program(os.Getenv, registerPlugins)))
}

// runOld runs one old command under a context the first SIGINT or SIGTERM ends and returns its exit code.
func runOld(args []string) int {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if err := dispatch(ctx, args, registerPlugins); err != nil {
		fmt.Fprintln(os.Stderr, "alphone:", err)
		return 1
	}
	return 0
}

// dispatch runs the old command the first argument names.
func dispatch(ctx context.Context, args []string, plugins func(sdk.Deps) ([]sdk.Plugin, error)) error {
	switch args[0] {
	case "createadmin":
		if err := declarePluginRoles(role.Default, os.Getenv, plugins); err != nil {
			return err
		}
		return createAdmin(ctx, os.Getenv, args[1:], os.Stdin, os.Stdout)
	case "grantrole":
		if err := declarePluginRoles(role.Default, os.Getenv, plugins); err != nil {
			return err
		}
		return grantRole(ctx, os.Getenv, args[1:], os.Stdout)
	}
	return token(ctx, os.Getenv, args[1:], os.Stdout)
}
