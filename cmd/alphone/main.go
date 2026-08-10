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

// usage is the help text the CLI prints when asked.
const usage = `AlphOne, a plugin first CRM.

Usage:
  alphone                serve the API and the web application
  alphone createadmin    create the first administrator
  alphone token          create and revoke API tokens
  alphone seed           store the demo data
  alphone help           print this text

Pass -h to a subcommand for its own flags.
Every setting is read from the environment, see
https://docs.alph.one/self-hosting/configuration/`

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
	case "help", "-h", "--help":
		_, err := fmt.Fprintln(os.Stdout, usage)
		return err
	default:
		return fmt.Errorf("%w %q, want createadmin, seed or token, or no argument to serve",
			errUnknownSubcommand, args[0])
	}
}
