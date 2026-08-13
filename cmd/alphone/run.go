// SPDX-License-Identifier: Elastic-2.0

package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/gopherium/gouncer/authkit"
	authkitpg "github.com/gopherium/gouncer/authkit/postgres"
	"github.com/gopherium/gouncer/authkit/ratelimit"

	"github.com/gopherium/pluginkit"

	"github.com/gopherium/alphone/internal/contact"
	"github.com/gopherium/alphone/internal/event"
	"github.com/gopherium/alphone/internal/graphres"
	"github.com/gopherium/alphone/internal/graphroot"
	"github.com/gopherium/alphone/internal/postgres"
	"github.com/gopherium/alphone/internal/server"
	"github.com/gopherium/alphone/internal/version"
	"github.com/gopherium/alphone/internal/webhook"
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

	settings, err := loadRunConfig(getenv)
	if err != nil {
		return err
	}

	pool, err := pgxpool.New(ctx, settings.databaseURL)
	if err != nil {
		return fmt.Errorf("parse database url: %w", err)
	}
	defer pool.Close()

	if err := migrateSchemas(ctx, settings.databaseURL); err != nil {
		return err
	}

	userStore := authkitpg.NewUserStore(pool)
	contacts := postgres.NewContactStore(pool)
	tasks := postgres.NewTaskStore(pool)
	tokens := postgres.NewTokenStore(pool)
	webhooks := postgres.NewWebhookStore(pool)
	dispatcher := webhook.NewDispatcher(webhooks, logger)
	deliveries := webhook.NewWorker(webhooks, logger)
	deliveries.Start()
	defer deliveries.Stop()
	hub := event.NewHub()
	events := nudgingPublisher{dispatcher: dispatcher, worker: deliveries, hub: hub}
	reaper := authkit.NewReaper(userStore, authkit.ReaperConfig{Logger: logger})
	reaper.Start()
	defer reaper.Stop()

	resolver := contact.NewResolver(contacts, contact.WithEvents(events))
	registered, err := plugins(sdk.Deps{
		DatabaseURL: settings.databaseURL,
		Resolver:    resolverBridge{resolver: resolver},
		Contacts:    directoryBridge{resolver: resolver},
		Events:      pluginPublisher{publisher: events},
		Getenv:      getenv,
	})
	if err != nil {
		return fmt.Errorf("register plugins: %w", err)
	}

	host := pluginkit.NewHost(registered...)
	if err := host.Start(ctx); err != nil {
		return fmt.Errorf("start plugins: %w", err)
	}

	auth := authkit.New(authkit.Config{Store: userStore, CookieName: server.SessionCookieName})
	admin := authkit.NewAdmin(userStore)
	graphRoot, err := graphroot.FromPlugins(&graphres.Resolver{
		Version:      version.Version(),
		Contacts:     contacts,
		Tasks:        tasks,
		Webhooks:     webhooks,
		Events:       events,
		Live:         hub,
		Auth:         auth,
		Admin:        admin,
		LoginLimiter: ratelimit.NewLimiter(ratelimit.Config{}),
	}, registered)
	if err != nil {
		return fmt.Errorf("compose graph root: %w", err)
	}

	cfg := server.Config{
		Version:           version.Version(),
		Users:             userStore,
		Auth:              auth,
		GraphRoot:         graphRoot,
		Tokens:            tokens,
		Plugins:           host.Routes(),
		PluginPublicPaths: host.PublicPaths(),
		FieldSources:      fieldSources(registered),
		TrustedProxies:    settings.trustedProxies,
		GraphiQL:          settings.graphiql,
	}
	if settings.webDir != "" {
		cfg.Web = os.DirFS(settings.webDir)
	}

	httpServer := &http.Server{
		Addr:              settings.addr,
		Handler:           server.NewServer(cfg),
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       30 * time.Second,
		IdleTimeout:       120 * time.Second,
	}
	return serveUntilDone(ctx, httpServer, host, logger)
}

// fieldSources returns every registered plugin serving runtime defined fields.
func fieldSources(registered []sdk.Plugin) []sdk.FieldSource {
	var sources []sdk.FieldSource
	for _, plugin := range registered {
		if source, ok := plugin.(sdk.FieldSource); ok {
			sources = append(sources, source)
		}
	}
	return sources
}

// runConfig carries the environment-derived settings of the server.
type runConfig struct {
	databaseURL    string
	addr           string
	webDir         string
	trustedProxies []string
	graphiql       bool
}

// loadRunConfig reads the server settings from the environment.
func loadRunConfig(getenv func(string) string) (runConfig, error) {
	databaseURL := getenv("ALPHONE_DATABASE_URL")
	if databaseURL == "" {
		return runConfig{}, errors.New("ALPHONE_DATABASE_URL is required")
	}
	addr := getenv("ALPHONE_ADDR")
	if addr == "" {
		addr = "localhost:8080"
	}
	trustedProxies, err := parseTrustedProxies(getenv("ALPHONE_TRUSTED_PROXIES"))
	if err != nil {
		return runConfig{}, err
	}
	return runConfig{
		databaseURL:    databaseURL,
		addr:           addr,
		webDir:         getenv("ALPHONE_WEB_DIR"),
		trustedProxies: trustedProxies,
		graphiql:       getenv("ALPHONE_DEV_GRAPHIQL") != "",
	}, nil
}

// serveUntilDone serves HTTP until ctx is cancelled or serving fails, then
// stops the plugin host.
func serveUntilDone(
	ctx context.Context,
	httpServer *http.Server,
	host *pluginkit.Host,
	logger *slog.Logger,
) error {
	serveErr := make(chan error, 1)
	go func() {
		serveErr <- httpServer.ListenAndServe()
	}()
	logger.Info("listening", "addr", httpServer.Addr)

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
	prefixes, err := ratelimit.ParseTrustedProxies(raw)
	if err != nil {
		return nil, fmt.Errorf("ALPHONE_TRUSTED_PROXIES: %w", err)
	}
	return prefixes, nil
}
