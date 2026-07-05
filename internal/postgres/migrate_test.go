// SPDX-License-Identifier: AGPL-3.0-or-later

package postgres_test

import (
	"database/sql"
	"testing"

	"github.com/peterldowns/pgtestdb"

	"github.com/gopherium/alphone/internal/postgres"
)

func TestMigrateCreatesCoreSchema(t *testing.T) {
	t.Parallel()
	if testing.Short() {
		t.Skip("skipping database test in short mode")
	}

	cfg := pgtestdb.Custom(t, testDBConfig(), pgtestdb.NoopMigrator{})

	if err := postgres.Migrate(t.Context(), cfg.URL()); err != nil {
		t.Fatalf("Migrate() error = %v, want nil", err)
	}
	if err := postgres.Migrate(t.Context(), cfg.URL()); err != nil {
		t.Fatalf("second Migrate() error = %v, want idempotent nil", err)
	}

	db, err := sql.Open("pgx", cfg.URL())
	if err != nil {
		t.Fatalf("opening database: %v", err)
	}
	defer db.Close()
	maria := mustContact(t, "María Pérez")
	if _, err := db.Exec("INSERT INTO core.contacts (id, name, created_at) VALUES ($1, $2, $3)", maria.ID, maria.Name, maria.CreatedAt); err != nil {
		t.Fatalf("inserting into migrated schema: %v", err)
	}
}

func TestMigrateRejectsMalformedURL(t *testing.T) {
	t.Parallel()

	if err := postgres.Migrate(t.Context(), "://not-a-url"); err == nil {
		t.Fatal("Migrate() error = nil, want a parse error")
	}
}

func TestMigrateReportsUnreachableDatabase(t *testing.T) {
	t.Parallel()

	err := postgres.Migrate(t.Context(), "postgres://postgres:alphone@localhost:9/postgres?sslmode=disable&connect_timeout=1")

	if err == nil {
		t.Fatal("Migrate() error = nil, want a connection error")
	}
}
