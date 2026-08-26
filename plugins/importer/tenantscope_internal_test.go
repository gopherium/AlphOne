// SPDX-License-Identifier: Elastic-2.0

package importer

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/gopherium/alphone/sdk"
)

// inTenant returns a context serving one freshly seeded tenant.
func inTenant(t *testing.T, pool *pgxpool.Pool) context.Context {
	t.Helper()
	id := uuid.Must(uuid.NewV7())
	if _, err := pool.Exec(t.Context(),
		"INSERT INTO core.tenants (id, name) VALUES ($1, $2)", id, "Acme"); err != nil {
		t.Fatalf("seeding the tenant: %v", err)
	}
	return sdk.WithTenant(t.Context(), id)
}

// stagedImport stores one import with a single row in the tenant the context serves.
func stagedImport(t *testing.T, s *store, ctx context.Context, filename string) uuid.UUID {
	t.Helper()
	stored, err := s.insertImport(ctx, uuid.Must(uuid.NewV7()), filename, sheet{
		columns: []string{"Name"},
		rows:    []row{{cells: []string{"Maria Perez"}}},
	})
	if err != nil {
		t.Fatalf("staging %s: %v", filename, err)
	}
	return stored.ID
}

func TestAnImportListingStaysInsideItsTenant(t *testing.T) {
	t.Parallel()

	pool := newMigratedPool(t)
	s := &store{pool: pool}
	acme := inTenant(t, pool)
	stagedImport(t, s, acme, "acme.csv")
	stagedImport(t, s, t.Context(), "elsewhere.csv")

	listed, err := s.listImports(acme)

	if err != nil {
		t.Fatalf("listImports() error = %v, want nil", err)
	}
	if len(listed) != 1 || listed[0].Filename != "acme.csv" {
		t.Errorf("listImports() = %+v, want only the tenant's own import", listed)
	}
}

func TestAnImportStaysUnreadableFromAnotherTenant(t *testing.T) {
	t.Parallel()

	pool := newMigratedPool(t)
	s := &store{pool: pool}
	acme := inTenant(t, pool)
	held := stagedImport(t, s, acme, "acme.csv")

	if _, err := s.importByID(acme, held); err != nil {
		t.Fatalf("importByID() inside the tenant error = %v, want the import", err)
	}
	if _, err := s.importByID(t.Context(), held); err == nil {
		t.Error("importByID() from another tenant answered, want the import withheld")
	}
}

func TestStagedRowsStayInsideTheirTenant(t *testing.T) {
	t.Parallel()

	pool := newMigratedPool(t)
	s := &store{pool: pool}
	acme := inTenant(t, pool)
	held := stagedImport(t, s, acme, "acme.csv")

	rows, err := s.listRows(t.Context(), held, 10)

	if err != nil {
		t.Fatalf("listRows() error = %v, want nil", err)
	}
	if len(rows) != 0 {
		t.Errorf("listRows() = %+v from another tenant, want the rows withheld", rows)
	}
}

func TestCommittingAnImportStaysInsideItsTenant(t *testing.T) {
	t.Parallel()

	pool := newMigratedPool(t)
	s := &store{pool: pool}
	acme := inTenant(t, pool)
	held := stagedImport(t, s, acme, "acme.csv")
	if err := s.updateMapping(acme, held, mapping{"0": "name"}); err != nil {
		t.Fatalf("updateMapping() error = %v, want nil", err)
	}

	if _, err := s.claimForCommit(t.Context(), held); err == nil {
		t.Error("claimForCommit() from another tenant succeeded, want the import withheld")
	}
}
