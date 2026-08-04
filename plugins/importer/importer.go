// SPDX-License-Identifier: Elastic-2.0

// Package importer brings contacts into the CRM from uploaded files.
package importer

import (
	"context"
	"database/sql"
	"embed"
	"fmt"
	"io/fs"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/jackc/pgx/v5/stdlib"
	"github.com/pressly/goose/v3"
	"github.com/pressly/goose/v3/database"

	"github.com/gopherium/alphone/sdk"
)

//go:embed migrations/*.sql
var migrations embed.FS

var migrationSource = mustSub(migrations, "migrations")

// Plugin imports contacts from CSV and Excel files.
type Plugin struct {
	pool     *pgxpool.Pool
	store    *store
	contacts sdk.ContactDirectory
	events   sdk.Publisher
}

// Register builds the importer [Plugin] from the host-provided deps.
func Register(deps sdk.Deps) (*Plugin, error) {
	pool, err := pgxpool.New(context.Background(), deps.DatabaseURL)
	if err != nil {
		return nil, fmt.Errorf("importer: connect database: %w", err)
	}
	return &Plugin{
		pool:     pool,
		store:    &store{pool: pool},
		contacts: deps.Contacts,
		events:   deps.Events,
	}, nil
}

// ID reports the plugin identifier.
func (p *Plugin) ID() string {
	return "importer"
}

// Start reports the plugin ready.
func (p *Plugin) Start(_ context.Context) error {
	return nil
}

// Stop releases the plugin's database resources.
func (p *Plugin) Stop(_ context.Context) error {
	p.pool.Close()
	return nil
}

// Migrate creates and updates the plugin-owned plugin_importer schema.
func (p *Plugin) Migrate(ctx context.Context) error {
	if _, err := p.pool.Exec(ctx, "CREATE SCHEMA IF NOT EXISTS plugin_importer"); err != nil {
		return fmt.Errorf("importer: create schema: %w", err)
	}
	db := stdlib.OpenDBFromPool(p.pool)
	defer func() { _ = db.Close() }()
	return migrate(ctx, db, "plugin_importer.goose_db_version")
}

// migrate applies the embedded goose migrations to db using the given version table.
func migrate(ctx context.Context, db *sql.DB, versionTable string) error {
	store, err := database.NewStore(database.DialectPostgres, versionTable)
	if err != nil {
		return fmt.Errorf("importer: migration store: %w", err)
	}
	provider, err := goose.NewProvider("", db, migrationSource, goose.WithStore(store))
	if err != nil {
		return fmt.Errorf("importer: migration provider: %w", err)
	}
	if _, err := provider.Up(ctx); err != nil {
		return fmt.Errorf("importer: apply migrations: %w", err)
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
