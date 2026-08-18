// SPDX-License-Identifier: Elastic-2.0

package testdb_test

import (
	"database/sql"
	"testing"

	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/peterldowns/pgtestdb"

	"github.com/gopherium/alphone/internal/testdb"
)

// migrated returns a database carrying every schema the migrator applies.
func migrated(t *testing.T) *sql.DB {
	t.Helper()
	if testing.Short() {
		t.Skip("skipping database test in short mode")
	}
	cfg := pgtestdb.Custom(t, testdb.Config(), testdb.Migrator())
	db, err := sql.Open("pgx", cfg.URL())
	if err != nil {
		t.Fatalf("opening the database: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return db
}

// tableExists reports whether the named table stands in the database.
func tableExists(t *testing.T, db *sql.DB, name string) bool {
	t.Helper()
	var present bool
	if err := db.QueryRow("SELECT to_regclass($1) IS NOT NULL", name).Scan(&present); err != nil {
		t.Fatalf("asking for %s: %v", name, err)
	}
	return present
}

func TestMigratorBuildsBothSchemas(t *testing.T) {
	t.Parallel()

	db := migrated(t)

	if !tableExists(t, db, "auth.users") {
		t.Error("auth.users is missing, the auth schema must migrate before core")
	}
	if !tableExists(t, db, "core.contacts") {
		t.Error("core.contacts is missing, the core schema must migrate")
	}
}

func TestMigratorLetsCoreMigrationsReachTheAuthSchema(t *testing.T) {
	t.Parallel()

	db := migrated(t)

	var users int
	if err := db.QueryRow("SELECT count(*) FROM auth.users").Scan(&users); err != nil {
		t.Fatalf("a core migration could not read auth.users: %v", err)
	}
}

func TestMigratorHashesBothMigrationSets(t *testing.T) {
	t.Parallel()

	hash, err := testdb.Migrator().Hash()

	if err != nil {
		t.Fatalf("Hash() error = %v, want nil", err)
	}
	if hash == "" {
		t.Error("Hash() is empty, want a template identity")
	}
	core, err := testdb.CoreMigrator().Hash()
	if err != nil {
		t.Fatalf("CoreMigrator().Hash() error = %v, want nil", err)
	}
	if hash == core {
		t.Error("Hash() matches the core only hash, want the auth migrations counted too")
	}
}
