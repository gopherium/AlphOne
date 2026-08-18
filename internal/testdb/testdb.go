// SPDX-License-Identifier: Elastic-2.0

// Package testdb provides shared PostgreSQL test-database configuration.
package testdb

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/peterldowns/pgtestdb"
	"github.com/peterldowns/pgtestdb/migrators/goosemigrator"

	authkitpg "github.com/gopherium/gouncer/authkit/postgres"

	"github.com/gopherium/alphone/internal/postgres"
)

// authVersionTable is where the auth schema records its own migrations.
const authVersionTable = "auth.goose_db_version"

// Config returns the pgtestdb configuration for the local compose database.
func Config() pgtestdb.Config {
	return pgtestdb.Config{
		DriverName: "pgx",
		User:       "postgres",
		Password:   "alphone",
		Host:       "localhost",
		Port:       "5433",
		Database:   "postgres",
		Options:    "sslmode=disable",
	}
}

// CoreMigrator returns a migrator applying the core schema migrations.
func CoreMigrator() *goosemigrator.GooseMigrator {
	return goosemigrator.New("migrations", goosemigrator.WithFS(postgres.Migrations))
}

// authMigrator returns a migrator applying the auth schema migrations.
func authMigrator() *goosemigrator.GooseMigrator {
	return goosemigrator.New("migrations",
		goosemigrator.WithFS(authkitpg.Migrations),
		goosemigrator.WithTableName(authVersionTable))
}

// schemaMigrator applies the auth schema before the core schema, the order the binary uses.
type schemaMigrator struct{}

// Migrator returns a migrator applying every schema one AlphOne database holds.
func Migrator() pgtestdb.Migrator {
	return schemaMigrator{}
}

// Hash returns the template identity of both migration sets together.
func (schemaMigrator) Hash() (string, error) {
	auth, err := authMigrator().Hash()
	if err != nil {
		return "", fmt.Errorf("testdb: hash the auth migrations: %w", err)
	}
	core, err := CoreMigrator().Hash()
	if err != nil {
		return "", fmt.Errorf("testdb: hash the core migrations: %w", err)
	}
	return auth + core, nil
}

// Migrate applies the auth schema and then the core schema to the template database.
func (schemaMigrator) Migrate(ctx context.Context, db *sql.DB, cfg pgtestdb.Config) error {
	if err := authkitpg.Migrate(ctx, cfg.URL()); err != nil {
		return fmt.Errorf("testdb: migrate the auth schema: %w", err)
	}
	if err := CoreMigrator().Migrate(ctx, db, cfg); err != nil {
		return fmt.Errorf("testdb: migrate the core schema: %w", err)
	}
	return nil
}
