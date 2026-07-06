// SPDX-License-Identifier: AGPL-3.0-or-later

// Command alphone runs the AlphOne CRM server.
package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/gopherium/alphone/internal/plugin"
	"github.com/gopherium/alphone/internal/postgres"
	"github.com/gopherium/alphone/internal/server"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if err := run(ctx, os.Getenv, os.Stderr, registerPlugins); err != nil {
		fmt.Fprintln(os.Stderr, "alphone:", err)
		os.Exit(1)
	}
}

func run(ctx context.Context, getenv func(string) string, stderr io.Writer, plugins func(*pgxpool.Pool, func(string) string) []plugin.Plugin) error {
	logger := slog.New(slog.NewTextHandler(stderr, nil))

	databaseURL := getenv("ALPHONE_DATABASE_URL")
	if databaseURL == "" {
		return errors.New("ALPHONE_DATABASE_URL is required")
	}
	addr := getenv("ALPHONE_ADDR")
	if addr == "" {
		addr = "localhost:8080"
	}

	pool, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		return fmt.Errorf("parse database url: %w", err)
	}
	defer pool.Close()

	if err := postgres.Migrate(ctx, databaseURL); err != nil {
		return err
	}

	host := plugin.NewHost(plugins(pool, getenv)...)
	if err := host.Start(ctx); err != nil {
		return fmt.Errorf("start plugins: %w", err)
	}

	httpServer := &http.Server{
		Addr:    addr,
		Handler: server.NewServer(postgres.NewContactStore(pool), host.Routes()),
	}
	serveErr := make(chan error, 1)
	go func() {
		serveErr <- httpServer.ListenAndServe()
	}()
	logger.Info("listening", "addr", addr)

	select {
	case err := <-serveErr:
		stopCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		return errors.Join(fmt.Errorf("http server: %w", err), host.Stop(stopCtx))
	case <-ctx.Done():
	}

	logger.Info("shutting down")
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	return errors.Join(httpServer.Shutdown(shutdownCtx), host.Stop(shutdownCtx))
}
