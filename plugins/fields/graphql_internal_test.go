// SPDX-License-Identifier: Elastic-2.0

package fields

import (
	"errors"
	"testing"

	"github.com/google/uuid"

	"github.com/gopherium/alphone/graph/model"
)

// errCatalogue is the failure a wedged catalogue reload reports.
var errCatalogue = errors.New("catalogue unavailable")

// newClosedPlugin returns a migrated plugin whose pool is already closed.
func newClosedPlugin(t *testing.T) *Plugin {
	t.Helper()
	p := newMigratedPlugin(t)
	p.pool.Close()
	return p
}

// newWedgedPlugin returns a working store whose catalogue always fails to reload.
func newWedgedPlugin(t *testing.T) *Plugin {
	t.Helper()
	p := newMigratedPlugin(t)
	p.catalog = newCatalog(&fakeLoader{err: errCatalogue})
	return p
}

func TestStoreReportsAClosedPool(t *testing.T) {
	t.Parallel()

	p := newClosedPlugin(t)

	if err := p.store.define(t.Context(), defined(t, "birthDate", "DATE")); err == nil {
		t.Error("create() error = nil, want the closed pool reported")
	}
	if err := p.store.archive(t.Context(), uuid.Must(uuid.NewV7())); err == nil {
		t.Error("archive() error = nil, want the closed pool reported")
	}
	if _, err := p.store.liveDefinitions(t.Context()); err == nil {
		t.Error("liveDefinitions() error = nil, want the closed pool reported")
	}
}

func TestResolversReportAClosedPool(t *testing.T) {
	t.Parallel()

	p := newClosedPlugin(t)

	if _, err := (QueryResolvers{plugin: p}).Fields(t.Context(), nil); err == nil {
		t.Error("Fields() error = nil, want the closed pool reported")
	}
	_, err := (MutationResolvers{plugin: p}).DefineField(t.Context(), "birthDate", "Birth date", model.FieldKindDate)
	if err == nil {
		t.Error("DefineField() error = nil, want the closed pool reported")
	}
	if _, err := (MutationResolvers{plugin: p}).ArchiveField(t.Context(), uuid.Must(uuid.NewV7())); err == nil {
		t.Error("ArchiveField() error = nil, want the closed pool reported")
	}
}

func TestResolversReportAFailedCatalogueReload(t *testing.T) {
	t.Parallel()

	p := newWedgedPlugin(t)
	stored := defined(t, "birthDate", "DATE")
	if err := p.store.define(t.Context(), stored); err != nil {
		t.Fatalf("create() error = %v, want nil", err)
	}

	_, err := (MutationResolvers{plugin: p}).DefineField(t.Context(), "loyaltyPoints", "Points", model.FieldKindNumber)
	if !errors.Is(err, errCatalogue) {
		t.Errorf("DefineField() error = %v, want the reload failure", err)
	}

	if _, err := (MutationResolvers{plugin: p}).ArchiveField(t.Context(), stored.ID); !errors.Is(err, errCatalogue) {
		t.Errorf("ArchiveField() error = %v, want the reload failure", err)
	}
}

func TestFieldsListsArchivedDefinitionsOnRequest(t *testing.T) {
	t.Parallel()

	p := newMigratedPlugin(t)
	stored := defined(t, "birthDate", "DATE")
	if err := p.store.define(t.Context(), stored); err != nil {
		t.Fatalf("create() error = %v, want nil", err)
	}
	if err := p.store.archive(t.Context(), stored.ID); err != nil {
		t.Fatalf("archive() error = %v, want nil", err)
	}
	wanted := true

	listed, err := (QueryResolvers{plugin: p}).Fields(t.Context(), &wanted)

	if err != nil {
		t.Fatalf("Fields() error = %v, want nil", err)
	}
	if len(listed) != 1 || listed[0].ArchivedAt == nil {
		t.Errorf("fields = %+v, want the archived definition listed", listed)
	}
}
