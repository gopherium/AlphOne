// SPDX-License-Identifier: Elastic-2.0

package email

import (
	"context"
	"embed"
	"errors"
	"fmt"
	"io/fs"

	dbkitpostgres "github.com/gopherium/framework/dbkit/postgres"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/jackc/pgx/v5/stdlib"
)

// uniqueViolation is the code PostgreSQL answers when another session stored the same key first.
const uniqueViolation = "23505"

// versionTable is where the plugin counts the migrations it applied.
const versionTable = "plugin_email.goose_db_version"

//go:embed migrations/*.sql
var migrations embed.FS

// migrationSource holds the plugin's migrations at its root.
var migrationSource = mustSub(migrations, "migrations")

// migrate creates the plugin_email schema and applies every pending migration under goose's session lock.
func migrate(ctx context.Context, pool *pgxpool.Pool) error {
	_, err := pool.Exec(ctx, "CREATE SCHEMA IF NOT EXISTS plugin_email")
	if err != nil && !isSchemaFromAnotherSession(err) {
		return fmt.Errorf("email: create schema: %w", err)
	}
	db := stdlib.OpenDBFromPool(pool)
	defer func() { _ = db.Close() }()
	err = dbkitpostgres.Migrate(ctx, db, dbkitpostgres.Migrations{Table: versionTable, FS: migrationSource})
	if err != nil {
		return fmt.Errorf("email: apply migrations: %w", err)
	}
	return nil
}

// isSchemaFromAnotherSession reports whether err says another session created the same schema first.
func isSchemaFromAnotherSession(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == uniqueViolation
}

// mustSub returns the sub-filesystem of fsys rooted at dir, panicking if it cannot be created.
func mustSub(fsys fs.FS, dir string) fs.FS {
	sub, err := fs.Sub(fsys, dir)
	if err != nil {
		panic(err)
	}
	return sub
}
