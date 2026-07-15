// SPDX-License-Identifier: Elastic-2.0

package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/netip"
	"os"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/gopherium/alphone"
	"github.com/gopherium/alphone/internal/contact"
	"github.com/gopherium/alphone/internal/plugin"
	"github.com/gopherium/alphone/internal/postgres"
	"github.com/gopherium/alphone/internal/server"
	"github.com/gopherium/alphone/sdk"
)

// run starts the server and serves until ctx is cancelled or serving fails.
func run(
	ctx context.Context,
	getenv func(string) string,
	stderr io.Writer,
	plugins func(sdk.Deps) ([]sdk.Plugin, error),
) error {
	logger := slog.New(slog.NewTextHandler(stderr, nil))

	databaseURL := getenv("ALPHONE_DATABASE_URL")
	if databaseURL == "" {
		return errors.New("ALPHONE_DATABASE_URL is required")
	}
	addr := getenv("ALPHONE_ADDR")
	if addr == "" {
		addr = "localhost:8080"
	}
	trustedProxies, err := parseTrustedProxies(getenv("ALPHONE_TRUSTED_PROXIES"))
	if err != nil {
		return err
	}

	pool, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		return fmt.Errorf("parse database url: %w", err)
	}
	defer pool.Close()

	if err := postgres.Migrate(ctx, databaseURL); err != nil {
		return err
	}

	userStore := postgres.NewUserStore(pool)
	contacts := postgres.NewContactStore(pool)
	reaperCtx, stopReaper := context.WithCancel(ctx)
	reaperDone := make(chan struct{})
	go func() {
		reapExpiredSessions(reaperCtx, userStore, sessionGCInterval, sessionGCTimeout, logger)
		close(reaperDone)
	}()
	defer func() {
		stopReaper()
		<-reaperDone
	}()

	registered, err := plugins(sdk.Deps{
		DatabaseURL: databaseURL,
		Resolver:    resolverBridge{resolver: contact.NewResolver(contacts)},
		Getenv:      getenv,
	})
	if err != nil {
		return fmt.Errorf("register plugins: %w", err)
	}

	host := plugin.NewHost(registered...)
	if err := host.Start(ctx); err != nil {
		return fmt.Errorf("start plugins: %w", err)
	}

	cfg := server.Config{
		Contacts:          contacts,
		Users:             userStore,
		Plugins:           host.Routes(),
		PluginPublicPaths: host.PublicPaths(),
		Version:           alphone.Version(),
		TrustedProxies:    trustedProxies,
	}
	if webDir := getenv("ALPHONE_WEB_DIR"); webDir != "" {
		cfg.Web = os.DirFS(webDir)
	}

	httpServer := &http.Server{
		Addr:              addr,
		Handler:           server.NewServer(cfg),
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       30 * time.Second,
		IdleTimeout:       120 * time.Second,
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

// parseTrustedProxies parses raw into trusted-proxy CIDR ranges.
func parseTrustedProxies(raw string) ([]string, error) {
	var prefixes []string
	for _, part := range strings.Split(raw, ",") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		if _, err := netip.ParsePrefix(part); err != nil {
			return nil, fmt.Errorf("ALPHONE_TRUSTED_PROXIES: invalid CIDR %q: %w", part, err)
		}
		prefixes = append(prefixes, part)
	}
	return prefixes, nil
}
