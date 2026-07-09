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

// main runs the alphone server.
func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	_ = godotenv.Load()
	if err := run(ctx, os.Getenv, os.Stderr, registerPlugins); err != nil {
		fmt.Fprintln(os.Stderr, "alphone:", err)
		os.Exit(1)
	}
}
