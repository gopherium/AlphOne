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
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/jackc/pgx/v5/stdlib"
	"github.com/pressly/goose/v3"
	"github.com/pressly/goose/v3/database"

	"github.com/gopherium/alphone/sdk"
)

//go:embed migrations/*.sql
var migrations embed.FS

var migrationSource = mustSub(migrations, "migrations")

// Plugin connects WhatsApp conversations to the CRM core.
type Plugin struct {
	pool        *pgxpool.Pool
	resolver    sdk.ContactResolver
	verifyToken string
	appSecret   string
	store       *store
	sender      *sender
	events      *broadcaster
}

// Register builds the WhatsApp [Plugin] from the host-provided deps,
// reading its Meta credentials from ALPHONE_WHATSAPP_VERIFY_TOKEN and
// ALPHONE_WHATSAPP_APP_SECRET.
func Register(deps sdk.Deps) (*Plugin, error) {
	getenv := deps.Getenv
	if getenv == nil {
		getenv = func(string) string { return "" }
	}
	pool, err := pgxpool.New(context.Background(), deps.DatabaseURL)
	if err != nil {
		return nil, fmt.Errorf("whatsapp: connect database: %w", err)
	}
	graphURL := getenv("ALPHONE_WHATSAPP_GRAPH_URL")
	if graphURL == "" {
		graphURL = defaultGraphURL
	}
	return &Plugin{
		pool:        pool,
		resolver:    deps.Resolver,
		verifyToken: getenv("ALPHONE_WHATSAPP_VERIFY_TOKEN"),
		appSecret:   getenv("ALPHONE_WHATSAPP_APP_SECRET"),
		store:       &store{pool: pool},
		sender: &sender{
			client:        &http.Client{Timeout: 10 * time.Second},
			baseURL:       graphURL,
			accessToken:   getenv("ALPHONE_WHATSAPP_ACCESS_TOKEN"),
			phoneNumberID: getenv("ALPHONE_WHATSAPP_PHONE_NUMBER_ID"),
		},
		events: newBroadcaster(),
	}, nil
}

// ID reports the plugin identifier.
func (p *Plugin) ID() string {
	return "whatsapp"
}

// Start is a placeholder until the plugin serves live traffic.
func (p *Plugin) Start(_ context.Context) error {
	return nil
}

// Stop releases the plugin's database resources.
func (p *Plugin) Stop(_ context.Context) error {
	p.pool.Close()
	return nil
}

// Routes returns the plugin's HTTP endpoints, served relative to its
// namespace.
func (p *Plugin) Routes() http.Handler {
	router := chi.NewRouter()
	router.Get("/webhook", p.handleVerify())
	router.Post("/webhook", p.handleEvents())
	router.Get("/conversations", p.handleConversationsList())
	router.Get("/conversations/{id}/messages", p.handleMessagesList())
	router.Post("/conversations/{id}/messages", p.handleMessageSend())
	router.Get("/events", p.handleStream())
	return router
}

// handleVerify returns a handler that answers Meta's webhook verification
// challenge by checking hub.verify_token and echoing hub.challenge.
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

// migrate applies the embedded goose migrations to db using the given version table.
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

// mustSub returns the sub-filesystem of fsys rooted at dir, panicking if it cannot be created.
func mustSub(fsys fs.FS, dir string) fs.FS {
	sub, err := fs.Sub(fsys, dir)
	if err != nil {
		panic(err)
	}
	return sub
}
