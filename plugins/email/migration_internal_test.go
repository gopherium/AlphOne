// SPDX-License-Identifier: Elastic-2.0

package email

import (
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/jackc/pgx/v5/stdlib"
	"github.com/pressly/goose/v3"
	"github.com/pressly/goose/v3/database"
)

// mailboxTables lists the tables the first migration creates.
var mailboxTables = []string{"plugin_email.mailboxes", "plugin_email.workspace_usage"}

// emailProvider returns a goose provider over the plugin's migrations and its own version table.
func emailProvider(t *testing.T, pool *pgxpool.Pool) *goose.Provider {
	t.Helper()
	db := stdlib.OpenDBFromPool(pool)
	t.Cleanup(func() { _ = db.Close() })
	store, err := database.NewStore(database.DialectPostgres, "plugin_email.goose_db_version")
	if err != nil {
		t.Fatalf("building the version store: %v", err)
	}
	provider, err := goose.NewProvider("", db, migrationSource, goose.WithStore(store))
	if err != nil {
		t.Fatalf("building the provider: %v", err)
	}
	return provider
}

// newestMigration returns the version of the newest migration file the plugin embeds.
func newestMigration(t *testing.T, pool *pgxpool.Pool) int64 {
	t.Helper()
	sources := emailProvider(t, pool).ListSources()
	if len(sources) == 0 {
		t.Fatal("the plugin embeds no migration, want its files listed")
	}
	return sources[len(sources)-1].Version
}

// emailTables counts the tables of the plugin_email schema, its version table left out.
func emailTables(t *testing.T, pool *pgxpool.Pool) int {
	t.Helper()
	var tables int
	if err := pool.QueryRow(t.Context(),
		"SELECT count(*) FROM pg_tables WHERE schemaname = 'plugin_email' AND tablename <> 'goose_db_version'",
	).Scan(&tables); err != nil {
		t.Fatalf("counting the plugin_email tables: %v", err)
	}
	return tables
}

func TestMigrateCreatesTheMailboxTables(t *testing.T) {
	t.Parallel()

	pool := newMigratedPool(t)

	for _, table := range mailboxTables {
		if !tableExists(t, pool, table) {
			t.Errorf("migrate() left out %s, want it created", table)
		}
	}
}

func TestMigrateCountsItsMigrationsInThePluginsOwnVersionTable(t *testing.T) {
	t.Parallel()

	pool := newMigratedPool(t)

	var newest int64
	if err := pool.QueryRow(t.Context(),
		"SELECT max(version_id) FROM plugin_email.goose_db_version").Scan(&newest); err != nil {
		t.Fatalf("reading plugin_email.goose_db_version: %v", err)
	}
	if want := newestMigration(t, pool); newest != want {
		t.Errorf("plugin_email.goose_db_version holds version %d, want the newest file's %d", newest, want)
	}
}

func TestMigrateAgainChangesNothing(t *testing.T) {
	t.Parallel()

	pool := newMigratedPool(t)
	migrated := emailTables(t, pool)

	if err := migrate(t.Context(), pool); err != nil {
		t.Fatalf("migrate() a second time error = %v, want nil", err)
	}
	if tables := emailTables(t, pool); tables != migrated {
		t.Errorf("plugin_email holds %d tables after a second migrate, want %d", tables, migrated)
	}
}

func TestTheMigrationsRollBackToNoTableAndUpAgain(t *testing.T) {
	t.Parallel()

	pool := newMigratedPool(t)
	migrated := emailTables(t, pool)
	provider := emailProvider(t, pool)

	if _, err := provider.DownTo(t.Context(), 0); err != nil {
		t.Fatalf("rolling every migration back: %v", err)
	}
	if tables := emailTables(t, pool); tables != 0 {
		t.Errorf("plugin_email holds %d tables after the rollback, want none", tables)
	}
	if _, err := provider.Up(t.Context()); err != nil {
		t.Fatalf("applying the migrations again: %v", err)
	}
	if tables := emailTables(t, pool); tables != migrated {
		t.Errorf("plugin_email holds %d tables after applying again, want %d", tables, migrated)
	}
}

func TestMustSubPanicsOnAFolderItCannotOpen(t *testing.T) {
	t.Parallel()

	defer func() {
		if recover() == nil {
			t.Fatal("mustSub(migrations, \"..\") did not panic, want a panic")
		}
	}()

	mustSub(migrations, "..")
}
