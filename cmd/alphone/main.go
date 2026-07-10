// SPDX-License-Identifier: Elastic-2.0

// Command alphone runs the AlphOne CRM server.
package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/joho/godotenv"
)

// main runs the alphone server, or one of its subcommands.
func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	_ = godotenv.Load()
	if len(os.Args) > 1 && os.Args[1] == "createadmin" {
		if err := createAdmin(ctx, os.Getenv, os.Args[2:], os.Stdin, os.Stdout); err != nil {
			fmt.Fprintln(os.Stderr, "alphone:", err)
			os.Exit(1)
		}
		return
	}
	if err := run(ctx, os.Getenv, os.Stderr, registerPlugins); err != nil {
		fmt.Fprintln(os.Stderr, "alphone:", err)
		os.Exit(1)
	}
}
