// SPDX-License-Identifier: AGPL-3.0-or-later

// Package testdb provides shared PostgreSQL test-database configuration.
package testdb

import (
	"github.com/peterldowns/pgtestdb"
	"github.com/peterldowns/pgtestdb/migrators/goosemigrator"

	"github.com/gopherium/alphone/internal/postgres"
)

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
