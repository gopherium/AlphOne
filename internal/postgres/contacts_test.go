// SPDX-License-Identifier: Elastic-2.0

package postgres_test

import (
	"errors"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/peterldowns/pgtestdb"

	"github.com/gopherium/alphone/internal/contact"
	"github.com/gopherium/alphone/internal/postgres"
	"github.com/gopherium/alphone/internal/testdb"
)

func newTestPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	if testing.Short() {
		t.Skip("skipping database test in short mode")
	}
	cfg := pgtestdb.Custom(t, testdb.Config(), testdb.CoreMigrator())
	pool, err := pgxpool.New(t.Context(), cfg.URL())
	if err != nil {
		t.Fatalf("connecting pool: %v", err)
	}
	t.Cleanup(pool.Close)
	return pool
}

func TestContactStoreRoundTrip(t *testing.T) {
	t.Parallel()

	store := postgres.NewContactStore(newTestPool(t))
	maria := mustContact(t, "María Pérez")

	if err := store.Create(t.Context(), maria); err != nil {
		t.Fatalf("Create() error = %v, want nil", err)
	}
	got, err := store.Get(t.Context(), maria.ID)

	if err != nil {
		t.Fatalf("Get() error = %v, want nil", err)
	}
	if diff := cmp.Diff(maria, got, cmpopts.EquateApproxTime(time.Microsecond)); diff != "" {
		t.Errorf("Get() mismatch (-want +got):\n%s", diff)
	}
}

func TestContactStoreGetMissingContact(t *testing.T) {
	t.Parallel()

	store := postgres.NewContactStore(newTestPool(t))

	_, err := store.Get(t.Context(), uuid.Must(uuid.NewV7()))

	if !errors.Is(err, contact.ErrNotFound) {
		t.Fatalf("Get() error = %v, want %v", err, contact.ErrNotFound)
	}
}

func TestContactStoreReportsConnectionFailure(t *testing.T) {
	t.Parallel()

	pool := newTestPool(t)
	store := postgres.NewContactStore(pool)
	maria := mustContact(t, "María Pérez")
	pool.Close()

	if err := store.Create(t.Context(), maria); err == nil {
		t.Error("Create() on closed pool error = nil, want error")
	}
	if _, err := store.Get(t.Context(), maria.ID); err == nil || errors.Is(err, contact.ErrNotFound) {
		t.Errorf("Get() on closed pool error = %v, want a non-ErrNotFound error", err)
	}
}
