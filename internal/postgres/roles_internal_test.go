// SPDX-License-Identifier: Elastic-2.0

package postgres

import (
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/peterldowns/pgtestdb"
)

// errWriteRefused reports a write the guarded transaction could not run.
var errWriteRefused = errors.New("write refused")

// probeConfig names the database server the guarded transaction tests run against.
func probeConfig() pgtestdb.Config {
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

// probePool returns a pool over a database migrated by the given migrator.
func probePool(t *testing.T, migrator pgtestdb.Migrator) *pgxpool.Pool {
	t.Helper()
	if testing.Short() {
		t.Skip("skipping database test in short mode")
	}
	cfg := pgtestdb.Custom(t, probeConfig(), migrator)
	pool, err := pgxpool.New(t.Context(), cfg.URL())
	if err != nil {
		t.Fatalf("connecting pool: %v", err)
	}
	t.Cleanup(pool.Close)
	return pool
}

// heldPool returns a pool over a database holding only the rows the guarded transaction locks.
func heldPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	pool := probePool(t, pgtestdb.NoopMigrator{})
	if _, err := pool.Exec(t.Context(), `CREATE SCHEMA core;
		CREATE TABLE core.user_roles (user_id uuid PRIMARY KEY, role text NOT NULL)`); err != nil {
		t.Fatalf("creating the roles table: %v", err)
	}
	return pool
}

func TestUnseatingReportsAWriteItCouldNotRun(t *testing.T) {
	t.Parallel()

	store := NewRoleStore(heldPool(t))

	err := store.unseating(t.Context(), func(pgx.Tx) error { return errWriteRefused })

	if !errors.Is(err, errWriteRefused) {
		t.Errorf("unseating() error = %v, want %v", err, errWriteRefused)
	}
}

func TestUnseatingReportsATransactionItCouldNotOpen(t *testing.T) {
	t.Parallel()

	pool := heldPool(t)
	store := NewRoleStore(pool)
	pool.Close()

	err := store.unseating(t.Context(), func(pgx.Tx) error { return nil })

	if err == nil {
		t.Error("unseating() on a closed pool error = nil, want error")
	}
}

func TestDisableReportsAGuardItCouldNotRead(t *testing.T) {
	t.Parallel()

	store := NewRoleStore(heldPool(t))

	err := store.Disable(t.Context(), uuid.Must(uuid.NewV7()))

	if err == nil {
		t.Error("Disable() error = nil on a database holding no users, want error")
	}
}

func TestUnseatingReportsAdminsItCouldNotHold(t *testing.T) {
	t.Parallel()

	store := NewRoleStore(probePool(t, pgtestdb.NoopMigrator{}))

	held := store.unseating(t.Context(), func(pgx.Tx) error { return nil })

	if held == nil {
		t.Error("unseating() error = nil on a database holding no roles, want error")
	}
}
