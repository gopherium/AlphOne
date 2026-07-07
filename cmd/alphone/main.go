// SPDX-License-Identifier: AGPL-3.0-or-later

// Command alphone runs the AlphOne CRM server.
package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if err := run(ctx, os.Getenv, os.Stderr, registerPlugins); err != nil {
		fmt.Fprintln(os.Stderr, "alphone:", err)
		os.Exit(1)
	}
}
