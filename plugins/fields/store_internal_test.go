// SPDX-License-Identifier: Elastic-2.0

package fields

import (
	"errors"
	"strings"
	"testing"

	"github.com/google/uuid"
	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/peterldowns/pgtestdb"

	"github.com/gopherium/alphone/internal/testdb"
	"github.com/gopherium/alphone/sdk"
)

// newMigratedPlugin returns a plugin over a database carrying the fields schema.
func newMigratedPlugin(t *testing.T) *Plugin {
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
	if err := p.Start(t.Context()); err != nil {
		t.Fatalf("Start() error = %v, want nil", err)
	}
	return p
}

func TestStoreRoundTripsADefinition(t *testing.T) {
	t.Parallel()

	p := newMigratedPlugin(t)
	definition := defined(t, "birthDate", "DATE")

	if err := p.store.define(t.Context(), definition); err != nil {
		t.Fatalf("define() error = %v, want nil", err)
	}

	held, err := p.store.liveDefinitions(t.Context())
	if err != nil {
		t.Fatalf("liveDefinitions() error = %v, want nil", err)
	}
	if len(held) != 1 {
		t.Fatalf("definitions = %d, want 1", len(held))
	}
	if held[0].Name != "birthDate" || held[0].Kind != kindDate || held[0].Label != "Label" {
		t.Errorf("definition = %+v, want the stored row", held[0])
	}
	if held[0].ArchivedAt != nil {
		t.Error("archivedAt is set, want a live definition")
	}
}

func TestStoreRefusesADuplicateName(t *testing.T) {
	t.Parallel()

	p := newMigratedPlugin(t)
	if err := p.store.define(t.Context(), defined(t, "birthDate", "DATE")); err != nil {
		t.Fatalf("define() error = %v, want nil", err)
	}

	err := p.store.define(t.Context(), defined(t, "birthDate", "TEXT"))

	if !errors.Is(err, errNameTaken) {
		t.Errorf("error = %v, want errNameTaken", err)
	}
}

func TestStoreRevivesAnArchivedDefinition(t *testing.T) {
	t.Parallel()

	p := newMigratedPlugin(t)
	original := defined(t, "birthDate", "DATE")
	if err := p.store.define(t.Context(), original); err != nil {
		t.Fatalf("define() error = %v, want nil", err)
	}
	if err := p.store.archive(t.Context(), original.ID); err != nil {
		t.Fatalf("archive() error = %v, want nil", err)
	}

	if err := p.store.define(t.Context(), defined(t, "birthDate", "DATE")); err != nil {
		t.Fatalf("define() error = %v, want the archived definition revived", err)
	}

	live, err := p.store.liveDefinitions(t.Context())
	if err != nil {
		t.Fatalf("liveDefinitions() error = %v, want nil", err)
	}
	if len(live) != 1 {
		t.Fatalf("live definitions = %d, want the revived one", len(live))
	}
	if live[0].ID != original.ID {
		t.Errorf("id = %s, want the original %s kept so stored values stay reachable", live[0].ID, original.ID)
	}
}

func TestStoreRefusesRevivingUnderADifferentKind(t *testing.T) {
	t.Parallel()

	p := newMigratedPlugin(t)
	original := defined(t, "birthDate", "DATE")
	if err := p.store.define(t.Context(), original); err != nil {
		t.Fatalf("define() error = %v, want nil", err)
	}
	if err := p.store.archive(t.Context(), original.ID); err != nil {
		t.Fatalf("archive() error = %v, want nil", err)
	}

	err := p.store.define(t.Context(), defined(t, "birthDate", "TEXT"))

	if !errors.Is(err, errKindLocked) {
		t.Errorf("error = %v, want errKindLocked", err)
	}
}

func TestStoreArchiveHidesADefinitionFromTheLiveListing(t *testing.T) {
	t.Parallel()

	p := newMigratedPlugin(t)
	definition := defined(t, "birthDate", "DATE")
	if err := p.store.define(t.Context(), definition); err != nil {
		t.Fatalf("define() error = %v, want nil", err)
	}

	if err := p.store.archive(t.Context(), definition.ID); err != nil {
		t.Fatalf("archive() error = %v, want nil", err)
	}

	live, err := p.store.liveDefinitions(t.Context())
	if err != nil {
		t.Fatalf("liveDefinitions() error = %v, want nil", err)
	}
	if len(live) != 0 {
		t.Errorf("live = %v, want the archived definition hidden", live)
	}
	every, err := p.store.allDefinitions(t.Context())
	if err != nil {
		t.Fatalf("allDefinitions() error = %v, want nil", err)
	}
	if len(every) != 1 || every[0].ArchivedAt == nil {
		t.Errorf("all = %+v, want the archived definition listed with its timestamp", every)
	}
}

func TestStoreArchiveRefusesAnUnknownID(t *testing.T) {
	t.Parallel()

	p := newMigratedPlugin(t)

	err := p.store.archive(t.Context(), uuid.Must(uuid.NewV7()))

	if !errors.Is(err, errNoDefinition) {
		t.Errorf("error = %v, want errNoDefinition", err)
	}
}

func TestStoreArchiveIsIdempotentOnALiveRow(t *testing.T) {
	t.Parallel()

	p := newMigratedPlugin(t)
	definition := defined(t, "birthDate", "DATE")
	if err := p.store.define(t.Context(), definition); err != nil {
		t.Fatalf("define() error = %v, want nil", err)
	}
	if err := p.store.archive(t.Context(), definition.ID); err != nil {
		t.Fatalf("first archive() error = %v, want nil", err)
	}

	err := p.store.archive(t.Context(), definition.ID)

	if !errors.Is(err, errNoDefinition) {
		t.Errorf("error = %v, want an already archived definition refused", err)
	}
}

func TestStoreReportsRowsItCannotRead(t *testing.T) {
	t.Parallel()

	p := newMigratedPlugin(t)

	_, err := p.store.query(t.Context(), "SELECT 1 WHERE $1::uuid IS NOT NULL")

	if err == nil {
		t.Fatal("query() error = nil, want the unreadable row reported")
	}
	if !strings.Contains(err.Error(), "read definitions") {
		t.Errorf("error = %v, want it to name the read", err)
	}
}

func TestStoreListsDefinitionsByCreation(t *testing.T) {
	t.Parallel()

	p := newMigratedPlugin(t)
	for _, name := range []string{"alpha", "beta", "gamma"} {
		if err := p.store.define(t.Context(), defined(t, name, "TEXT")); err != nil {
			t.Fatalf("define(%q) error = %v, want nil", name, err)
		}
	}

	held, err := p.store.liveDefinitions(t.Context())

	if err != nil {
		t.Fatalf("liveDefinitions() error = %v, want nil", err)
	}
	for i, want := range []string{"alpha", "beta", "gamma"} {
		if held[i].Name != want {
			t.Errorf("definition %d = %q, want %q in creation order", i, held[i].Name, want)
		}
	}
}
