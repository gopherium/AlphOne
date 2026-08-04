// SPDX-License-Identifier: Elastic-2.0

package importer

import (
	"database/sql"
	"testing"

	_ "github.com/jackc/pgx/v5/stdlib"
)

func TestMigrateRequiresVersionTable(t *testing.T) {
	t.Parallel()

	if err := migrate(t.Context(), nil, ""); err == nil {
		t.Fatal("migrate() error = nil, want a store error")
	}
}

func TestMigrateRequiresDatabase(t *testing.T) {
	t.Parallel()

	if err := migrate(t.Context(), nil, "plugin_importer.goose_db_version"); err == nil {
		t.Fatal("migrate(nil) error = nil, want a provider error")
	}
}

func TestMigrateReportsUnreachableDatabase(t *testing.T) {
	t.Parallel()

	db, err := sql.Open("pgx", "postgres://postgres:alphone@localhost:9/postgres?sslmode=disable&connect_timeout=1")
	if err != nil {
		t.Fatalf("opening database: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	if err := migrate(t.Context(), db, "plugin_importer.goose_db_version"); err == nil {
		t.Fatal("migrate() on unreachable database error = nil, want an error")
	}
}

func TestMustSubRejectsInvalidDir(t *testing.T) {
	t.Parallel()

	defer func() {
		if recover() == nil {
			t.Fatal("mustSub(..) did not panic, want a panic")
		}
	}()
	mustSub(migrations, "..")
}
