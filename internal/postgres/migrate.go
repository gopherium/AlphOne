// SPDX-License-Identifier: Elastic-2.0

package postgres

import (
	"context"
	"database/sql"
	"fmt"
	"io/fs"

	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/pressly/goose/v3"
	"github.com/pressly/goose/v3/lock"
)

var migrationSource = mustSub(Migrations, "migrations")

// restampIdentities marks each identity another server numbered, or numbered past this server's count, added now.
const restampIdentities = `UPDATE core.contact_identities SET added_xid = DEFAULT, added_server = DEFAULT
WHERE added_server <> (pg_control_system()).system_identifier
    OR added_xid >= pg_snapshot_xmax(pg_current_snapshot())`

// Migrate applies the core schema migrations to the database at databaseURL.
func Migrate(ctx context.Context, databaseURL string) error {
	return migrateDatabase(ctx, "pgx", databaseURL)
}

// migrateDatabase opens a database connection using driverName and databaseURL and applies the migrations.
func migrateDatabase(ctx context.Context, driverName, databaseURL string) error {
	db, err := sql.Open(driverName, databaseURL)
	if err != nil {
		return fmt.Errorf("postgres: open database: %w", err)
	}
	defer func() { _ = db.Close() }()
	return migrate(ctx, db)
}

// migrate runs all pending up migrations against db under goose's session lock, then restamps the identities.
func migrate(ctx context.Context, db *sql.DB) error {
	locker := mustLocker(lock.NewPostgresSessionLocker())
	provider, err := goose.NewProvider(goose.DialectPostgres, db, migrationSource, goose.WithSessionLocker(locker))
	if err != nil {
		return fmt.Errorf("postgres: migration provider: %w", err)
	}
	if _, err := provider.Up(ctx); err != nil {
		return fmt.Errorf("postgres: apply migrations: %w", err)
	}
	if _, err := db.ExecContext(ctx, restampIdentities); err != nil {
		return fmt.Errorf("postgres: restamp the identities: %w", err)
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

// mustSub returns the dir subtree of fsys and panics if it cannot be created.
func mustSub(fsys fs.FS, dir string) fs.FS {
	sub, err := fs.Sub(fsys, dir)
	if err != nil {
		panic(err)
	}
	return sub
}
