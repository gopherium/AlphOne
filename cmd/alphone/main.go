// SPDX-License-Identifier: Elastic-2.0

// Command alphone runs the AlphOne CRM server.
package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/joho/godotenv"
)

// errUnknownSubcommand reports a first argument naming no subcommand.
var errUnknownSubcommand = errors.New("unknown subcommand")

// main runs the alphone server, or one of its subcommands.
func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	_ = godotenv.Load()
	if err := dispatch(ctx, os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "alphone:", err)
		os.Exit(1)
	}
}

// dispatch runs the subcommand named by the first argument, or the server.
func dispatch(ctx context.Context, args []string) error {
	if len(args) == 0 {
		return run(ctx, os.Getenv, os.Stderr, registerPlugins)
	}
	switch args[0] {
	case "createadmin":
		return createAdmin(ctx, os.Getenv, args[1:], os.Stdin, os.Stdout)
	case "token":
		return token(ctx, os.Getenv, args[1:], os.Stdout)
	case "seed":
		return seed(ctx, os.Getenv, os.Stdout)
	default:
		return fmt.Errorf("%w %q, want createadmin, seed or token, or no argument to serve",
			errUnknownSubcommand, args[0])
	}
}
