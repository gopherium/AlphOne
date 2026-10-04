// SPDX-License-Identifier: Elastic-2.0

package whatsapp

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/peterldowns/pgtestdb"
	"github.com/pressly/goose/v3/lock"

	"github.com/gopherium/alphone/internal/testdb"
	"github.com/gopherium/alphone/sdk"
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

// newUnmigratedPlugin returns a plugin over a fresh database without the whatsapp schema, beside its address.
func newUnmigratedPlugin(t *testing.T) (*Plugin, string) {
	t.Helper()
	if testing.Short() {
		t.Skip("skipping database test in short mode")
	}
	address := pgtestdb.Custom(t, testdb.Config(), testdb.Migrator()).URL()
	p, err := Register(sdk.Deps{DatabaseURL: address})
	if err != nil {
		t.Fatalf("Register() error = %v, want nil", err)
	}
	t.Cleanup(func() { _ = p.Stop(context.Background()) })
	return p, address
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

// tableExists reports whether the database behind conn holds the table named name.
func tableExists(t *testing.T, conn *pgx.Conn, name string) bool {
	t.Helper()
	var present bool
	if err := conn.QueryRow(t.Context(), "SELECT to_regclass($1) IS NOT NULL", name).Scan(&present); err != nil {
		t.Fatalf("asking for %s: %v", name, err)
	}
	return present
}

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
			t.Fatalf("Migrate() = %v before it waited on the schema of the rival session", err)
		case <-time.After(rivalPoll):
		}
	}
}

func TestMustLockerPanicsOnAnError(t *testing.T) {
	t.Parallel()

	failed := errors.New("no locker")
	defer func() {
		if recovered := recover(); recovered != failed {
			t.Fatalf("mustLocker() recovered %v, want a panic with the error", recovered)
		}
	}()

	mustLocker(nil, failed)
}

func TestMigrateWaitsForTheMigrationLock(t *testing.T) {
	t.Parallel()

	p, address := newUnmigratedPlugin(t)
	rival := sessionTo(t, address)
	if _, err := rival.Exec(t.Context(), "SELECT pg_advisory_lock($1)", lock.DefaultLockID); err != nil {
		t.Fatalf("holding the migration lock: %v", err)
	}
	ctx, cancel := context.WithTimeout(t.Context(), lockedMigrateBudget)
	defer cancel()

	err := p.Migrate(ctx)

	if !errors.Is(err, context.DeadlineExceeded) || tableExists(t, rival, "plugin_whatsapp.conversations") {
		t.Errorf("Migrate() = %v, want the deadline and no plugin_whatsapp.conversations table", err)
	}
}

func TestMigrateSucceedsWhenAnotherSessionCreatedTheSchemaFirst(t *testing.T) {
	t.Parallel()

	p, address := newUnmigratedPlugin(t)
	rival, observer := sessionTo(t, address), sessionTo(t, address)
	ctx, cancel := context.WithTimeout(t.Context(), rivalBudget)
	defer cancel()
	tx, err := rival.Begin(ctx)
	if err != nil {
		t.Fatalf("beginning the rival transaction: %v", err)
	}
	if _, err := tx.Exec(ctx, "CREATE SCHEMA plugin_whatsapp"); err != nil {
		t.Fatalf("creating the plugin_whatsapp schema in the rival session: %v", err)
	}
	ran := make(chan error, 1)
	go func() { ran <- p.Migrate(ctx) }()
	awaitLockWait(ctx, t, observer, ran)

	if err := tx.Commit(ctx); err != nil {
		t.Fatalf("committing the schema of the rival session: %v", err)
	}

	if err := <-ran; err != nil || !tableExists(t, observer, "plugin_whatsapp.conversations") {
		t.Errorf("Migrate() = %v, want it to succeed and create the plugin_whatsapp.conversations table", err)
	}
}

func TestMigrateReportsASchemaCreateTheDatabaseRefuses(t *testing.T) {
	t.Parallel()

	p, address := newUnmigratedPlugin(t)
	session := sessionTo(t, address)
	database := pgx.Identifier{session.Config().Database}.Sanitize()
	readOnly := "ALTER DATABASE " + database + " SET default_transaction_read_only = on"
	if _, err := session.Exec(t.Context(), readOnly); err != nil {
		t.Fatalf("making the database read only: %v", err)
	}

	err := p.Migrate(t.Context())

	if err == nil || !strings.HasPrefix(err.Error(), "whatsapp: create schema: ") {
		t.Errorf("Migrate() = %v, want the schema create named", err)
	}
}
