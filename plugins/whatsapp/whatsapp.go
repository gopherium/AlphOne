// SPDX-License-Identifier: Elastic-2.0

// Package whatsapp ingests WhatsApp messages into the CRM.
package whatsapp

import (
	"context"
	"crypto/subtle"
	"database/sql"
	"embed"
	"errors"
	"fmt"
	"io/fs"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/jackc/pgx/v5/stdlib"
	"github.com/pressly/goose/v3"
	"github.com/pressly/goose/v3/database"
	"github.com/pressly/goose/v3/lock"

	"github.com/gopherium/alphone/sdk"
)

// uniqueViolation is the code Postgres answers when another session stored the same key first.
const uniqueViolation = "23505"

//go:embed migrations/*.sql
var migrations embed.FS

var migrationSource = mustSub(migrations, "migrations")

// Plugin connects WhatsApp conversations to the CRM core.
type Plugin struct {
	pool           *pgxpool.Pool
	resolver       sdk.ContactResolver
	verifyToken    string
	appSecret      string
	key            []byte
	envCredentials credentials
	gate           sdk.TenantGate
	store          *store
	sender         *sender
	events         *broadcaster
	fetcher        *mediaFetcher
	publisher      sdk.Publisher
}

// transportTemplate is the pool settings every outbound client is cloned from.
var transportTemplate = http.DefaultTransport.(*http.Transport)

// newOutboundClient returns a client calling over its own connection pool.
func newOutboundClient(timeout time.Duration) *http.Client {
	return &http.Client{Timeout: timeout, Transport: transportTemplate.Clone()}
}

// Register builds the WhatsApp [Plugin] from the host-provided deps.
func Register(deps sdk.Deps) (*Plugin, error) {
	env := deps.Env.Within("WHATSAPP_")
	maxBytes, err := env.Count("MEDIA_MAX_BYTES", defaultMediaMaxBytes)
	if err != nil {
		return nil, err
	}
	key, err := sdk.Parse(env, "CREDENTIALS_KEY", nil, credentialsKey)
	if err != nil {
		return nil, err
	}
	pool, err := pgxpool.New(context.Background(), deps.DatabaseURL)
	if err != nil {
		return nil, fmt.Errorf("whatsapp: connect database: %w", err)
	}
	graphURL := env.Value("GRAPH_URL")
	if graphURL == "" {
		graphURL = defaultGraphURL
	}
	p := &Plugin{
		pool:        pool,
		resolver:    deps.Resolver,
		publisher:   deps.Events,
		verifyToken: env.Value("VERIFY_TOKEN"),
		appSecret:   env.Value("APP_SECRET"),
		key:         key,
		envCredentials: credentials{
			phoneNumberID: env.Value("PHONE_NUMBER_ID"),
			accessToken:   env.Value("ACCESS_TOKEN"),
		},
		store: &store{pool: pool},
		sender: &sender{
			client:  newOutboundClient(10 * time.Second),
			baseURL: graphURL,
		},
		events: newBroadcaster(),
	}
	p.fetcher = newMediaFetcher(p.store, p.events, mediaFetcherConfig{
		baseURL:     graphURL,
		credentials: p.credentialsFor,
		records:     p.recordsTraffic,
		maxBytes:    int64(maxBytes),
	})
	return p, nil
}

// ID reports the plugin identifier.
func (p *Plugin) ID() string {
	return "whatsapp"
}

// Start launches the plugin's media download loop.
func (p *Plugin) Start(_ context.Context) error {
	p.fetcher.Start()
	return nil
}

// Stop halts the media download loop and closes the database pool, returning the error of ctx when ctx ends first.
func (p *Plugin) Stop(ctx context.Context) error {
	if err := p.fetcher.Stop(ctx); err != nil {
		go p.pool.Close()
		return fmt.Errorf("whatsapp: stop media fetcher: %w", err)
	}
	closed := make(chan struct{})
	go func() {
		defer close(closed)
		p.pool.Close()
	}()
	select {
	case <-closed:
		return nil
	case <-ctx.Done():
		return fmt.Errorf("whatsapp: close database pool: %w", ctx.Err())
	}
}

// Routes returns the plugin's HTTP endpoints, served relative to its
// namespace.
func (p *Plugin) Routes() http.Handler {
	router := chi.NewRouter()
	router.Get("/webhook", p.handleVerify())
	router.Post("/webhook", p.handleEvents())
	router.Get("/conversations/{id}/messages/{mid}/media", p.handleMediaDownload())
	return router
}

// Area names the scope area the plugin's protected routes act in.
func (p *Plugin) Area() string {
	return "whatsapp"
}

// PublicPaths declares the webhook as reachable without a login session;
// Meta authenticates it with its own signature instead.
func (p *Plugin) PublicPaths() []string {
	return []string{"/webhook"}
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
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(query.Get("hub.challenge")))
	}
}

// Migrate creates and updates the plugin-owned plugin_whatsapp schema.
func (p *Plugin) Migrate(ctx context.Context) error {
	_, err := p.pool.Exec(ctx, "CREATE SCHEMA IF NOT EXISTS plugin_whatsapp")
	if err != nil && !isSchemaFromAnotherSession(err) {
		return fmt.Errorf("whatsapp: create schema: %w", err)
	}
	db := stdlib.OpenDBFromPool(p.pool)
	defer func() { _ = db.Close() }()
	return migrate(ctx, db, "plugin_whatsapp.goose_db_version")
}

// isSchemaFromAnotherSession reports whether err says another session created the same schema first.
func isSchemaFromAnotherSession(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == uniqueViolation
}

// migrate applies the embedded goose migrations to db using the given version table under goose's session lock.
func migrate(ctx context.Context, db *sql.DB, versionTable string) error {
	store, err := database.NewStore(database.DialectPostgres, versionTable)
	if err != nil {
		return fmt.Errorf("whatsapp: migration store: %w", err)
	}
	locker := mustLocker(lock.NewPostgresSessionLocker())
	provider, err := goose.NewProvider("", db, migrationSource, goose.WithStore(store), goose.WithSessionLocker(locker))
	if err != nil {
		return fmt.Errorf("whatsapp: migration provider: %w", err)
	}
	if _, err := provider.Up(ctx); err != nil {
		return fmt.Errorf("whatsapp: apply migrations: %w", err)
	}
	return nil
}

// mustLocker returns locker and panics if goose could not build it.
func mustLocker(locker lock.SessionLocker, err error) lock.SessionLocker {
	if err != nil {
		panic(err)
	}
	return locker
}

// mustSub returns the sub-filesystem of fsys rooted at dir, panicking if it cannot be created.
func mustSub(fsys fs.FS, dir string) fs.FS {
	sub, err := fs.Sub(fsys, dir)
	if err != nil {
		panic(err)
	}
	return sub
}
