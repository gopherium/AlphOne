// SPDX-License-Identifier: Elastic-2.0

// Package fields serves contact fields an operator defines while AlphOne runs.
package fields

import (
	"context"
	"database/sql"
	"embed"
	"fmt"
	"io/fs"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/jackc/pgx/v5/stdlib"
	"github.com/pressly/goose/v3"
	"github.com/pressly/goose/v3/database"

	"github.com/gopherium/alphone/sdk"
)

//go:embed migrations/*.sql
var migrations embed.FS

var migrationSource = mustSub(migrations, "migrations")

// Plugin holds the catalogue of contact fields an operator defines.
type Plugin struct {
	pool      *pgxpool.Pool
	store     *store
	catalog   *catalog
	batchWait time.Duration
}

// Register builds the fields [Plugin] from the host-provided deps.
func Register(deps sdk.Deps) (*Plugin, error) {
	pool, err := pgxpool.New(context.Background(), deps.DatabaseURL)
	if err != nil {
		return nil, fmt.Errorf("fields: connect database: %w", err)
	}
	store := &store{pool: pool}
	return &Plugin{pool: pool, store: store, catalog: newCatalog(store)}, nil
}

// ID reports the plugin identifier.
func (p *Plugin) ID() string {
	return "fields"
}

// Start loads the catalogue the graph serves.
func (p *Plugin) Start(ctx context.Context) error {
	return p.catalog.reload(ctx)
}

// Stop releases the plugin's database resources.
func (p *Plugin) Stop(_ context.Context) error {
	p.pool.Close()
	return nil
}

// Migrate creates and updates the plugin-owned plugin_fields schema.
func (p *Plugin) Migrate(ctx context.Context) error {
	if _, err := p.pool.Exec(ctx, "CREATE SCHEMA IF NOT EXISTS plugin_fields"); err != nil {
		return fmt.Errorf("fields: create schema: %w", err)
	}
	db := stdlib.OpenDBFromPool(p.pool)
	defer func() { _ = db.Close() }()
	return migrate(ctx, db, "plugin_fields.goose_db_version")
}

// migrate applies the embedded goose migrations to db using the given version table.
func migrate(ctx context.Context, db *sql.DB, versionTable string) error {
	store, err := database.NewStore(database.DialectPostgres, versionTable)
	if err != nil {
		return fmt.Errorf("fields: migration store: %w", err)
	}
	provider, err := goose.NewProvider("", db, migrationSource, goose.WithStore(store))
	if err != nil {
		return fmt.Errorf("fields: migration provider: %w", err)
	}
	if _, err := provider.Up(ctx); err != nil {
		return fmt.Errorf("fields: apply migrations: %w", err)
	}
	return nil
}

// FieldsSnapshot reports the catalogue version and the fields the graph serves.
func (p *Plugin) FieldsSnapshot(_ context.Context) (uint64, []sdk.GraphField, error) {
	version, held := p.catalog.snapshot()
	return version, held, nil
}

// mustSub returns the sub-filesystem of fsys rooted at dir, panicking if it cannot be created.
func mustSub(fsys fs.FS, dir string) fs.FS {
	sub, err := fs.Sub(fsys, dir)
	if err != nil {
		panic(err)
	}
	return sub
}
