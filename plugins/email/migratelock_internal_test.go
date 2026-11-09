// SPDX-License-Identifier: Elastic-2.0

package email

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/pressly/goose/v3/lock"
)

// lockedMigrateBudget is how long a migration may try for a lock that a rival session keeps.
const lockedMigrateBudget = 2 * time.Second

// rivalBudget bounds every wait of a test that migrates beside a rival session.
const rivalBudget = 30 * time.Second

// rivalPoll is the pause between two looks at the sessions of the test database.
const rivalPoll = 10 * time.Millisecond

// lockWaitLookup asks whether another session of the same database waits on a lock.
const lockWaitLookup = "SELECT EXISTS (SELECT FROM pg_stat_activity WHERE datname = current_database()" +
	" AND pid <> pg_backend_pid() AND wait_event_type = 'Lock')"

// awaitLockWait returns once another session of the observer's database waits on a lock, failing if ran ends first.
func awaitLockWait(ctx context.Context, t *testing.T, observer *pgx.Conn, ran <-chan error) {
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
			t.Fatalf("migrate() = %v before it waited on the schema of the rival session", err)
		case <-time.After(rivalPoll):
		}
	}
}

func TestMigrateWaitsForTheMigrationLock(t *testing.T) {
	t.Parallel()

	pool, address := newUnmigratedPool(t)
	rival := sessionTo(t, address)
	if _, err := rival.Exec(t.Context(), "SELECT pg_advisory_lock($1)", lock.DefaultLockID); err != nil {
		t.Fatalf("holding the migration lock: %v", err)
	}
	ctx, cancel := context.WithTimeout(t.Context(), lockedMigrateBudget)
	defer cancel()

	err := migrate(ctx, pool)

	if !errors.Is(err, context.DeadlineExceeded) || tableExists(t, rival, "plugin_email.mailboxes") {
		t.Errorf("migrate() = %v, want the deadline and no plugin_email.mailboxes table", err)
	}
	if err == nil || !strings.HasPrefix(err.Error(), "email: apply migrations: ") {
		t.Errorf("migrate() = %v, want the migrations named", err)
	}
}

func TestMigrateSucceedsWhenAnotherSessionCreatedTheSchemaFirst(t *testing.T) {
	t.Parallel()

	pool, address := newUnmigratedPool(t)
	rival, observer := sessionTo(t, address), sessionTo(t, address)
	ctx, cancel := context.WithTimeout(t.Context(), rivalBudget)
	defer cancel()
	tx, err := rival.Begin(ctx)
	if err != nil {
		t.Fatalf("beginning the rival transaction: %v", err)
	}
	if _, err := tx.Exec(ctx, "CREATE SCHEMA plugin_email"); err != nil {
		t.Fatalf("creating the plugin_email schema in the rival session: %v", err)
	}
	ran := make(chan error, 1)
	go func() { ran <- migrate(ctx, pool) }()
	awaitLockWait(ctx, t, observer, ran)

	if err := tx.Commit(ctx); err != nil {
		t.Fatalf("committing the schema of the rival session: %v", err)
	}

	if err := <-ran; err != nil || !tableExists(t, observer, "plugin_email.mailboxes") {
		t.Errorf("migrate() = %v, want it to succeed and create the plugin_email.mailboxes table", err)
	}
}

func TestMigrateReportsASchemaCreateTheDatabaseRefuses(t *testing.T) {
	t.Parallel()

	pool, address := newUnmigratedPool(t)
	session := sessionTo(t, address)
	database := pgx.Identifier{session.Config().Database}.Sanitize()
	readOnly := "ALTER DATABASE " + database + " SET default_transaction_read_only = on"
	if _, err := session.Exec(t.Context(), readOnly); err != nil {
		t.Fatalf("making the database read only: %v", err)
	}

	err := migrate(t.Context(), pool)

	if err == nil || !strings.HasPrefix(err.Error(), "email: create schema: ") {
		t.Errorf("migrate() = %v, want the schema create named", err)
	}
}
