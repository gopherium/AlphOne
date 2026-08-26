// SPDX-License-Identifier: Elastic-2.0

package importer

import (
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/peterldowns/pgtestdb"

	"github.com/gopherium/alphone/internal/testdb"
	"github.com/gopherium/alphone/sdk"
)

var errEntropy = errors.New("entropy source failed")

type failingReader struct{}

// Read returns the entropy failure instead of any bytes.
func (failingReader) Read([]byte) (int, error) {
	return 0, errEntropy
}

// newMigratedPool returns a pool over a database carrying the importer schema.
func newMigratedPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	if testing.Short() {
		t.Skip("skipping database test in short mode")
	}
	cfg := pgtestdb.Custom(t, testdb.Config(), testdb.Migrator())
	p, err := Register(sdk.Deps{DatabaseURL: cfg.URL()})
	if err != nil {
		t.Fatalf("Register() error = %v, want nil", err)
	}
	t.Cleanup(func() { _ = p.Stop(t.Context()) })
	if err := p.Migrate(t.Context()); err != nil {
		t.Fatalf("Migrate() error = %v, want nil", err)
	}
	return p.pool
}

func TestInsertImportReportsIDGenerationFailure(t *testing.T) {
	uuid.SetRand(failingReader{})
	defer uuid.SetRand(nil)

	s := &store{}

	_, err := s.insertImport(t.Context(), uuid.Nil, "contacts.csv", sheet{})

	if !errors.Is(err, errEntropy) {
		t.Fatalf("insertImport() error = %v, want %v", err, errEntropy)
	}
}

func TestWriteImportReportsARowFailure(t *testing.T) {
	t.Parallel()

	pool := newMigratedPool(t)
	s := &store{pool: pool}
	shared := uuid.Must(uuid.NewV7())
	ids := []uuid.UUID{uuid.Must(uuid.NewV7()), shared, shared}
	parsed := sheet{
		columns: []string{"Name"},
		rows:    []row{{cells: []string{"Maria Perez"}}, {cells: []string{"Ana Lopez"}}},
	}

	_, err := s.writeImport(t.Context(), ids, uuid.Must(uuid.NewV7()), "contacts.csv", parsed)

	if err == nil {
		t.Fatal("writeImport() with a duplicate row id error = nil, want a key violation")
	}
	var stored int
	if err := pool.QueryRow(t.Context(),
		"SELECT count(*) FROM plugin_importer.imports").Scan(&stored); err != nil {
		t.Fatalf("counting imports: %v", err)
	}
	if stored != 0 {
		t.Errorf("stored %d imports, want the transaction rolled back", stored)
	}
}

func TestWriteImportReportsConnectionFailure(t *testing.T) {
	t.Parallel()

	pool, err := pgxpool.New(t.Context(), "postgres://postgres:alphone@localhost:5433/postgres?sslmode=disable")
	if err != nil {
		t.Fatalf("connecting pool: %v", err)
	}
	pool.Close()
	s := &store{pool: pool}

	if _, err := s.writeImport(t.Context(), []uuid.UUID{uuid.Nil}, uuid.Nil, "contacts.csv", sheet{}); err == nil {
		t.Fatal("writeImport() on closed pool error = nil, want an error")
	}
}
