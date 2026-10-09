// SPDX-License-Identifier: Elastic-2.0

package main

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/pressly/goose/v3"
	"github.com/pressly/goose/v3/lock"

	"github.com/gopherium/framework/gonsole/testkit"

	"github.com/gopherium/alphone/sdk"
)

// lockedMigrateBudget is how long a migration may try for a lock that a rival session keeps.
const lockedMigrateBudget = 2 * time.Second

// rivalBudget bounds every wait of a test that migrates beside a rival session.
const rivalBudget = 30 * time.Second

// rivalPoll is the pause between two looks at the sessions of the test database.
const rivalPoll = 10 * time.Millisecond

// pluginStopGrace bounds the stop of every plugin a test registered.
const pluginStopGrace = 10 * time.Second

// pluginSchemaPrefix starts the name of the schema each plugin owns.
const pluginSchemaPrefix = "plugin_"

// lockWaitLookup asks whether another session of the same database waits on a lock.
const lockWaitLookup = "SELECT EXISTS (SELECT FROM pg_stat_activity WHERE datname = current_database()" +
	" AND pid <> pg_backend_pid() AND wait_event_type = 'Lock')"

// pluginTablesLookup counts the tables of the schema named $1 apart from the version table named $2.
const pluginTablesLookup = "SELECT count(*) FROM pg_tables WHERE schemaname = $1 AND tablename <> $2"

// registeredOver registers the generated plugin list over the database at address and stops each plugin at the end.
func registeredOver(t *testing.T, address string) []sdk.Plugin {
	t.Helper()
	registered, err := registerPlugins(
		sdk.Deps{DatabaseURL: address, Getenv: testkit.Getenv(nil), Env: settingsEnv(testkit.Getenv(nil))})
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), pluginStopGrace)
		defer cancel()
		for _, plugin := range registered {
			if err := plugin.Stop(ctx); err != nil {
				t.Errorf("stopping plugin %s: %v", plugin.ID(), err)
			}
		}
	})
	if err != nil {
		t.Fatalf("registerPlugins() error = %v, want nil", err)
	}
	return registered
}

// schemaOf names the schema a migrating plugin owns, its id after the prefix with each hyphen written as an underscore.
func schemaOf(plugin sdk.Plugin) string {
	return pluginSchemaPrefix + strings.ReplaceAll(plugin.ID(), "-", "_")
}

// sessionTo returns a session of its own on the database at address, closed when the test ends.
func sessionTo(t *testing.T, address string) *pgx.Conn {
	t.Helper()
	conn, err := pgx.Connect(t.Context(), address)
	if err != nil {
		t.Fatalf("connecting a session: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close(context.Background()) })
	return conn
}

// pluginTables returns how many tables the schema named schema holds apart from goose's version table.
func pluginTables(t *testing.T, conn *pgx.Conn, schema string) int {
	t.Helper()
	var tables int
	if err := conn.QueryRow(t.Context(), pluginTablesLookup, schema, goose.DefaultTablename).Scan(&tables); err != nil {
		t.Fatalf("counting the tables of %s: %v", schema, err)
	}
	return tables
}

// awaitLockWait returns once another session of the observer's database waits on a lock, failing if ran ends first.
func awaitLockWait(ctx context.Context, t *testing.T, observer *pgx.Conn, ran <-chan error, plugin sdk.Plugin) {
	t.Helper()
	for {
		var waiting bool
		if err := observer.QueryRow(ctx, lockWaitLookup).Scan(&waiting); err != nil {
			t.Fatalf("looking for a session that waits on a lock: %v", err)
		}
		if waiting {
			return
		}
		select {
		case err := <-ran:
			t.Fatalf("plugin %s: Migrate() = %v before it waited on the %s schema of the rival session, "+
				"want the plugin to own that schema", plugin.ID(), err, schemaOf(plugin))
		case <-time.After(rivalPoll):
		}
	}
}

// checkWaitsForTheMigrationLock pins that the plugin at index creates no table while a rival holds goose's lock.
func checkWaitsForTheMigrationLock(t *testing.T, index int) {
	address := testDatabaseURL(t)
	plugin := registeredOver(t, address)[index]
	schema := schemaOf(plugin)
	rival := sessionTo(t, address)
	if _, err := rival.Exec(t.Context(), "SELECT pg_advisory_lock($1)", lock.DefaultLockID); err != nil {
		t.Fatalf("holding the migration lock: %v", err)
	}
	ctx, cancel := context.WithTimeout(t.Context(), lockedMigrateBudget)
	defer cancel()

	err := plugin.(sdk.Migrator).Migrate(ctx)

	if tables := pluginTables(t, rival, schema); !errors.Is(err, context.DeadlineExceeded) || tables != 0 {
		t.Errorf("plugin %s: Migrate() = %v with %d tables in %s, want the deadline and no table",
			plugin.ID(), err, tables, schema)
	}
}

// checkSucceedsAfterARivalSchema pins that the plugin at index migrates once a rival session commits its schema.
func checkSucceedsAfterARivalSchema(t *testing.T, index int) {
	address := testDatabaseURL(t)
	plugin := registeredOver(t, address)[index]
	schema := schemaOf(plugin)
	rival, observer := sessionTo(t, address), sessionTo(t, address)
	ctx, cancel := context.WithTimeout(t.Context(), rivalBudget)
	defer cancel()
	tx, err := rival.Begin(ctx)
	if err != nil {
		t.Fatalf("beginning the rival transaction: %v", err)
	}
	if _, err := tx.Exec(ctx, "CREATE SCHEMA "+pgx.Identifier{schema}.Sanitize()); err != nil {
		t.Fatalf("creating the %s schema in the rival session: %v", schema, err)
	}
	ran := make(chan error, 1)
	go func() { ran <- plugin.(sdk.Migrator).Migrate(ctx) }()
	awaitLockWait(ctx, t, observer, ran, plugin)

	if err := tx.Commit(ctx); err != nil {
		t.Fatalf("committing the schema of the rival session: %v", err)
	}

	if err := <-ran; err != nil || pluginTables(t, observer, schema) == 0 {
		t.Errorf("plugin %s: Migrate() = %v, want it to succeed and fill %s once the rival session commits",
			plugin.ID(), err, schema)
	}
}

func TestEveryRegisteredPluginMigratesUnderGoosesSessionLock(t *testing.T) {
	t.Parallel()

	checked := 0
	for index, plugin := range registeredOver(t, "") {
		if _, migrates := plugin.(sdk.Migrator); !migrates {
			continue
		}
		checked++
		t.Run(plugin.ID(), func(t *testing.T) {
			t.Parallel()
			t.Run("waits for the migration lock", func(t *testing.T) {
				t.Parallel()
				checkWaitsForTheMigrationLock(t, index)
			})
			t.Run("succeeds when another session created the schema first", func(t *testing.T) {
				t.Parallel()
				checkSucceedsAfterARivalSchema(t, index)
			})
		})
	}
	if checked == 0 {
		t.Fatal("no registered plugin migrates, so the guard checks nothing")
	}
}
