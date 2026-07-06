// SPDX-License-Identifier: AGPL-3.0-or-later

// Package whatsapp ingests WhatsApp messages into the CRM.
package whatsapp

import (
	"context"
	"crypto/subtle"
	"database/sql"
	"embed"
	"fmt"
	"io/fs"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/jackc/pgx/v5/stdlib"
	"github.com/pressly/goose/v3"
	"github.com/pressly/goose/v3/database"
)

//go:embed migrations/*.sql
var migrations embed.FS

var migrationSource = mustSub(migrations, "migrations")

// Config holds the Meta credentials the plugin needs.
type Config struct {
	VerifyToken string
	AppSecret   string
}

// Plugin connects WhatsApp conversations to the CRM core.
type Plugin struct {
	pool        *pgxpool.Pool
	resolver    ContactResolver
	verifyToken string
	appSecret   string
	store       *store
}

// New returns the WhatsApp [Plugin] backed by pool, resolving inbound
// senders through resolver.
func New(pool *pgxpool.Pool, resolver ContactResolver, cfg Config) *Plugin {
	return &Plugin{
		pool:        pool,
		resolver:    resolver,
		verifyToken: cfg.VerifyToken,
		appSecret:   cfg.AppSecret,
		store:       &store{pool: pool},
	}
}

// ID reports the plugin identifier.
func (p *Plugin) ID() string {
	return "whatsapp"
}

// Start is a placeholder until the plugin serves live traffic.
func (p *Plugin) Start(_ context.Context) error {
	return nil
}

// Stop is a placeholder until the plugin holds running work.
func (p *Plugin) Stop(_ context.Context) error {
	return nil
}

// Routes returns the plugin's HTTP endpoints, served relative to its
// namespace.
func (p *Plugin) Routes() http.Handler {
	router := chi.NewRouter()
	router.Get("/webhook", p.handleVerify())
	router.Post("/webhook", p.handleEvents())
	return router
}

func (p *Plugin) handleVerify() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		query := r.URL.Query()
		tokenMatches := subtle.ConstantTimeCompare([]byte(query.Get("hub.verify_token")), []byte(p.verifyToken)) == 1
		if p.verifyToken == "" || query.Get("hub.mode") != "subscribe" || !tokenMatches {
			w.WriteHeader(http.StatusForbidden)
			return
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(query.Get("hub.challenge")))
	}
}

// Migrate creates and updates the plugin-owned plugin_whatsapp schema.
func (p *Plugin) Migrate(ctx context.Context) error {
	if _, err := p.pool.Exec(ctx, "CREATE SCHEMA IF NOT EXISTS plugin_whatsapp"); err != nil {
		return fmt.Errorf("whatsapp: create schema: %w", err)
	}
	db := stdlib.OpenDBFromPool(p.pool)
	defer func() { _ = db.Close() }()
	return migrate(ctx, db, "plugin_whatsapp.goose_db_version")
}

func migrate(ctx context.Context, db *sql.DB, versionTable string) error {
	store, err := database.NewStore(database.DialectPostgres, versionTable)
	if err != nil {
		return fmt.Errorf("whatsapp: migration store: %w", err)
	}
	provider, err := goose.NewProvider("", db, migrationSource, goose.WithStore(store))
	if err != nil {
		return fmt.Errorf("whatsapp: migration provider: %w", err)
	}
	if _, err := provider.Up(ctx); err != nil {
		return fmt.Errorf("whatsapp: apply migrations: %w", err)
	}
	return nil
}

func mustSub(fsys fs.FS, dir string) fs.FS {
	sub, err := fs.Sub(fsys, dir)
	if err != nil {
		panic(err)
	}
	return sub
}
