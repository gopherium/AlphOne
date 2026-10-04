// SPDX-License-Identifier: Elastic-2.0

package postgres_test

import (
	"context"
	"database/sql"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/peterldowns/pgtestdb"
	"github.com/pressly/goose/v3/lock"

	authkitpg "github.com/gopherium/gouncer/authkit/postgres"

	"github.com/gopherium/alphone/internal/postgres"
	"github.com/gopherium/alphone/internal/testdb"
)

const (
	heldLockDeadline = 2 * time.Second
	runsAtOnce       = 2
	runsAtOnceBound  = time.Minute
)

// authMigrated returns the address of a fresh database holding only the auth schema the core schema needs.
func authMigrated(t *testing.T) string {
	t.Helper()
	if testing.Short() {
		t.Skip("skipping database test in short mode")
	}
	cfg := pgtestdb.Custom(t, testdb.Config(), pgtestdb.NoopMigrator{})
	if err := authkitpg.Migrate(t.Context(), cfg.URL()); err != nil {
		t.Fatalf("migrating the auth schema: %v", err)
	}
	return cfg.URL()
}

// openDatabase returns a handle on the database at databaseURL that is closed when the test ends.
func openDatabase(t *testing.T, databaseURL string) *sql.DB {
	t.Helper()
	db, err := sql.Open("pgx", databaseURL)
	if err != nil {
		t.Fatalf("opening database: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return db
}

// holdMigrationLock takes goose's migration lock on a session of db that lasts until the test ends.
func holdMigrationLock(t *testing.T, db *sql.DB) {
	t.Helper()
	holder, err := db.Conn(t.Context())
	if err != nil {
		t.Fatalf("taking a connection: %v", err)
	}
	t.Cleanup(func() { _ = holder.Close() })
	if _, err := holder.ExecContext(t.Context(), "SELECT pg_advisory_lock($1)", lock.DefaultLockID); err != nil {
		t.Fatalf("holding the migration lock: %v", err)
	}
}

// coreSchemaExists reports whether db holds the core schema that the first core migration creates.
func coreSchemaExists(t *testing.T, db *sql.DB) bool {
	t.Helper()
	var created bool
	if err := db.QueryRow("SELECT to_regnamespace('core') IS NOT NULL").Scan(&created); err != nil {
		t.Fatalf("looking for the core schema: %v", err)
	}
	return created
}

// applyFirstStep applies only the first core migration to db, leaving the version table in place and the rest pending.
func applyFirstStep(t *testing.T, db *sql.DB) {
	t.Helper()
	if _, err := newCoreProvider(t, db).UpByOne(t.Context()); err != nil {
		t.Fatalf("applying the first core migration: %v", err)
	}
}

// coreSteps returns how many migrations the core schema ships.
func coreSteps(t *testing.T, db *sql.DB) int {
	t.Helper()
	return len(newCoreProvider(t, db).ListSources())
}

// recordedSteps returns how many rows and how many distinct versions goose recorded for the core migrations in db.
func recordedSteps(t *testing.T, db *sql.DB) (rows, versions int) {
	t.Helper()
	err := db.QueryRow(
		"SELECT count(*), count(DISTINCT version_id) FROM public.goose_db_version WHERE version_id > 0",
	).Scan(&rows, &versions)
	if err != nil {
		t.Fatalf("counting the recorded migrations: %v", err)
	}
	return rows, versions
}

func TestMigrateCreatesCoreSchema(t *testing.T) {
	t.Parallel()
	if testing.Short() {
		t.Skip("skipping database test in short mode")
	}

	cfg := pgtestdb.Custom(t, testdb.Config(), pgtestdb.NoopMigrator{})
	if err := authkitpg.Migrate(t.Context(), cfg.URL()); err != nil {
		t.Fatalf("migrating the auth schema: %v", err)
	}

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
	defer func() { _ = db.Close() }()
	maria := mustContact(t, "María Pérez")
	if _, err := db.Exec(
		"INSERT INTO core.contacts (id, name, created_at) VALUES ($1, $2, $3)",
		maria.ID,
		maria.Name,
		maria.CreatedAt,
	); err != nil {
		t.Fatalf("inserting into migrated schema: %v", err)
	}
}

func TestMigrateWaitsForTheMigrationLock(t *testing.T) {
	t.Parallel()

	databaseURL := authMigrated(t)
	db := openDatabase(t, databaseURL)
	holdMigrationLock(t, db)
	ctx, cancel := context.WithTimeout(t.Context(), heldLockDeadline)
	defer cancel()

	err := postgres.Migrate(ctx, databaseURL)

	if created := coreSchemaExists(t, db); !errors.Is(err, context.DeadlineExceeded) || created {
		t.Errorf("Migrate() error = %v with the core schema created %v, want the deadline and no schema", err, created)
	}
}

func TestMigrateAppliesEachPendingStepOnceWhenTwoRunAtOnce(t *testing.T) {
	t.Parallel()

	databaseURL := authMigrated(t)
	db := openDatabase(t, databaseURL)
	applyFirstStep(t, db)
	ctx, cancel := context.WithTimeout(t.Context(), runsAtOnceBound)
	defer cancel()
	failures := make(chan error, runsAtOnce)
	var runs sync.WaitGroup
	for range runsAtOnce {
		runs.Go(func() { failures <- postgres.Migrate(ctx, databaseURL) })
	}
	runs.Wait()
	close(failures)

	for err := range failures {
		if err != nil {
			t.Errorf("Migrate() error = %v, want both runs to succeed", err)
		}
	}
	steps := coreSteps(t, db)
	rows, versions := recordedSteps(t, db)
	if rows != steps || versions != steps {
		t.Errorf("goose recorded %d rows over %d versions, want each of the %d steps once", rows, versions, steps)
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

	err := postgres.Migrate(
		t.Context(),
		"postgres://postgres:alphone@localhost:9/postgres?sslmode=disable&connect_timeout=1",
	)

	if err == nil {
		t.Fatal("Migrate() error = nil, want a connection error")
	}
}
