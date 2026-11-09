// SPDX-License-Identifier: Elastic-2.0

package email

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/gopherium/alphone/internal/testdb"
)

// errEntropy is the failure of a random source that answers no bytes.
var errEntropy = errors.New("entropy source failed")

// failingReader is a random source that answers errEntropy on every read.
type failingReader struct{}

// Read returns errEntropy instead of any bytes.
func (failingReader) Read([]byte) (int, error) {
	return 0, errEntropy
}

// newUnmigratedPool returns a pool over a fresh database holding the core schema alone, beside its address.
func newUnmigratedPool(t *testing.T) (*pgxpool.Pool, string) {
	t.Helper()
	if testing.Short() {
		t.Skip("skipping database test in short mode")
	}
	address := testdb.New(t, testdb.Migrator())
	pool, err := pgxpool.New(context.Background(), address)
	if err != nil {
		t.Fatalf("opening a pool over the test database: %v", err)
	}
	t.Cleanup(pool.Close)
	return pool, address
}

// newMigratedPool returns a pool over a fresh database holding the core schema and the email schema.
func newMigratedPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	pool, _ := newUnmigratedPool(t)
	if err := migrate(t.Context(), pool); err != nil {
		t.Fatalf("migrate() error = %v, want nil", err)
	}
	return pool
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

// rowReader reads one row on a pool or a session.
type rowReader interface {
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
}

// tableExists reports whether the database behind db holds the table named name.
func tableExists(t *testing.T, db rowReader, name string) bool {
	t.Helper()
	var present bool
	if err := db.QueryRow(t.Context(), "SELECT to_regclass($1) IS NOT NULL", name).Scan(&present); err != nil {
		t.Fatalf("asking for %s: %v", name, err)
	}
	return present
}

// storeWorkspace stores one more workspace, named Acme, and returns its id.
func storeWorkspace(t *testing.T, pool *pgxpool.Pool) uuid.UUID {
	t.Helper()
	id := uuid.Must(uuid.NewV7())
	if _, err := pool.Exec(t.Context(), "INSERT INTO core.tenants (id, name) VALUES ($1, 'Acme')", id); err != nil {
		t.Fatalf("storing a second workspace: %v", err)
	}
	return id
}

// beginTx opens a transaction on pool that rolls back when the test ends unless it ended first.
func beginTx(t *testing.T, pool *pgxpool.Pool) pgx.Tx {
	t.Helper()
	tx, err := pool.Begin(t.Context())
	if err != nil {
		t.Fatalf("beginning a transaction: %v", err)
	}
	t.Cleanup(func() { _ = tx.Rollback(context.Background()) })
	return tx
}
