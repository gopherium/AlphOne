// SPDX-License-Identifier: Elastic-2.0

// Package fields serves contact fields an operator defines while AlphOne runs.
package fields

import (
	"context"
	"database/sql"
	"embed"
	"errors"
	"fmt"
	"io/fs"
	"math"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/jackc/pgx/v5/stdlib"
	"github.com/pressly/goose/v3"
	"github.com/pressly/goose/v3/database"
	"github.com/pressly/goose/v3/lock"

	"github.com/gopherium/alphone/sdk"
)

//go:embed migrations/*.sql
var migrations embed.FS

var migrationSource = mustSub(migrations, "migrations")

// entriesMaxSetting names the setting under the program prefix that caps the entries one repeater holds.
const entriesMaxSetting = "FIELDS_ENTRIES_MAX"

// defaultEntriesMax is how many entries one repeater holds when the setting is unset.
const defaultEntriesMax = 500

// uniqueViolation is the code Postgres answers when another session stored the same key first.
const uniqueViolation = "23505"

// Plugin holds the catalogue of contact fields an operator defines.
type Plugin struct {
	pool       *pgxpool.Pool
	store      *store
	catalog    *catalog
	batchWait  time.Duration
	entriesMax int
}

// Register builds the fields [Plugin] from the host-provided deps.
func Register(deps sdk.Deps) (*Plugin, error) {
	entriesMax, err := deps.Env.Count(entriesMaxSetting, defaultEntriesMax, sdk.AtMost(math.MaxInt32))
	if err != nil {
		return nil, err
	}
	pool, err := pgxpool.New(context.Background(), deps.DatabaseURL)
	if err != nil {
		return nil, fmt.Errorf("fields: connect database: %w", err)
	}
	store := &store{pool: pool}
	return &Plugin{
		pool: pool, store: store, entriesMax: entriesMax,
		catalog: newCatalog(store, deps.TenantsHeld, deps.TenantsRefresh),
	}, nil
}

// ID reports the plugin identifier.
func (p *Plugin) ID() string {
	return "fields"
}

// Start leaves every tenant's catalogue to the first request that needs it.
func (p *Plugin) Start(_ context.Context) error {
	return nil
}

// Stop releases the plugin's database resources.
func (p *Plugin) Stop(_ context.Context) error {
	p.pool.Close()
	return nil
}

// Migrate creates and updates the plugin-owned plugin_fields schema.
func (p *Plugin) Migrate(ctx context.Context) error {
	_, err := p.pool.Exec(ctx, "CREATE SCHEMA IF NOT EXISTS plugin_fields")
	if err != nil && !isSchemaFromAnotherSession(err) {
		return fmt.Errorf("fields: create schema: %w", err)
	}
	db := stdlib.OpenDBFromPool(p.pool)
	defer func() { _ = db.Close() }()
	return migrate(ctx, db, "plugin_fields.goose_db_version")
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
		return fmt.Errorf("fields: migration store: %w", err)
	}
	locker := mustLocker(lock.NewPostgresSessionLocker())
	provider, err := goose.NewProvider("", db, migrationSource, goose.WithStore(store), goose.WithSessionLocker(locker))
	if err != nil {
		return fmt.Errorf("fields: migration provider: %w", err)
	}
	if _, err := provider.Up(ctx); err != nil {
		return fmt.Errorf("fields: apply migrations: %w", err)
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

// FieldsSnapshot reports the stamp of the calling tenant's catalogue and the fields the graph serves it.
func (p *Plugin) FieldsSnapshot(ctx context.Context) (uint64, []sdk.GraphField, error) {
	read, err := p.catalog.viewFor(ctx)
	if err != nil {
		return 0, nil, err
	}
	return read.stamp, read.fields, nil
}

// mustSub returns the sub-filesystem of fsys rooted at dir, panicking if it cannot be created.
func mustSub(fsys fs.FS, dir string) fs.FS {
	sub, err := fs.Sub(fsys, dir)
	if err != nil {
		panic(err)
	}
	return sub
}
