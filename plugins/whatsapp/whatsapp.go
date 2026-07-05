// SPDX-License-Identifier: AGPL-3.0-or-later

// Package whatsapp ingests WhatsApp messages into the CRM.
package whatsapp

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
)

//go:embed migrations/*.sql
var migrations embed.FS

var migrationSource = mustSub(migrations, "migrations")

// Plugin connects WhatsApp conversations to the CRM core.
type Plugin struct {
	pool *pgxpool.Pool
}

// New returns the WhatsApp [Plugin] backed by pool.
func New(pool *pgxpool.Pool) *Plugin {
	return &Plugin{pool: pool}
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
