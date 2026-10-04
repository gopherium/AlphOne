// SPDX-License-Identifier: Elastic-2.0

// Command alphone runs the AlphOne CRM server and the commands an operator runs beside it.
package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/joho/godotenv"

	"github.com/gopherium/framework/gonsole"
)

// main runs the alphone command line, the old token command through its own dispatch.
func main() {
	_ = godotenv.Load()
	if len(os.Args) > 1 && os.Args[1] == "token" {
		os.Exit(runToken(os.Args[2:]))
	}
	os.Exit(gonsole.Main(program(os.Getenv, registerPlugins)))
}

// runToken runs the old token command under a context the first SIGINT or SIGTERM ends and returns its exit code.
func runToken(args []string) int {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if err := token(ctx, os.Getenv, args, os.Stdout); err != nil {
		fmt.Fprintln(os.Stderr, "alphone:", err)
		return 1
	}
	return 0
}
