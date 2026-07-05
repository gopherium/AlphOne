// SPDX-License-Identifier: AGPL-3.0-or-later

package postgres

import (
	"context"
	"database/sql"
	"fmt"
	"io/fs"

	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/pressly/goose/v3"
)

var migrationSource = mustSub(Migrations, "migrations")

// Migrate applies the core schema migrations to the database at databaseURL.
func Migrate(ctx context.Context, databaseURL string) error {
	return migrateDatabase(ctx, "pgx", databaseURL)
}

func migrateDatabase(ctx context.Context, driverName, databaseURL string) error {
	db, err := sql.Open(driverName, databaseURL)
	if err != nil {
		return fmt.Errorf("postgres: open database: %w", err)
	}
	defer func() { _ = db.Close() }()
	return migrate(ctx, db)
}

func migrate(ctx context.Context, db *sql.DB) error {
	provider, err := goose.NewProvider(goose.DialectPostgres, db, migrationSource)
	if err != nil {
		return fmt.Errorf("postgres: migration provider: %w", err)
	}
	if _, err := provider.Up(ctx); err != nil {
		return fmt.Errorf("postgres: apply migrations: %w", err)
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
