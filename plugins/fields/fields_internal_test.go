// SPDX-License-Identifier: Elastic-2.0

package fields

import (
	"database/sql"
	"testing"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"

	"github.com/gopherium/alphone/sdk"
)

func TestIDNamesThePlugin(t *testing.T) {
	t.Parallel()

	if got := (&Plugin{}).ID(); got != "fields" {
		t.Errorf("ID() = %q, want fields", got)
	}
}

func TestRegisterRejectsAMalformedDatabaseURL(t *testing.T) {
	t.Parallel()

	if _, err := Register(sdk.Deps{DatabaseURL: "://nonsense"}); err == nil {
		t.Fatal("Register() error = nil, want a connection error")
	}
}

func TestMigrateRequiresVersionTable(t *testing.T) {
	t.Parallel()

	if err := migrate(t.Context(), nil, ""); err == nil {
		t.Fatal("migrate() error = nil, want a store error")
	}
}

func TestMigrateRequiresDatabase(t *testing.T) {
	t.Parallel()

	if err := migrate(t.Context(), nil, "plugin_fields.goose_db_version"); err == nil {
		t.Fatal("migrate(nil) error = nil, want a provider error")
	}
}

func TestMigrateReportsUnreachableDatabase(t *testing.T) {
	t.Parallel()

	db, err := sql.Open("pgx", "postgres://postgres:alphone@localhost:9/postgres?sslmode=disable&connect_timeout=1")
	if err != nil {
		t.Fatalf("opening database: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	if err := migrate(t.Context(), db, "plugin_fields.goose_db_version"); err == nil {
		t.Fatal("migrate() on unreachable database error = nil, want an error")
	}
}

func TestMigrateReportsAnUnreachableSchemaCreate(t *testing.T) {
	t.Parallel()

	p, err := Register(sdk.Deps{DatabaseURL: "postgres://plugin:plugin@localhost:9/plugin?connect_timeout=1"})
	if err != nil {
		t.Fatalf("Register() error = %v, want nil", err)
	}
	t.Cleanup(func() { _ = p.Stop(t.Context()) })

	if err := p.Migrate(t.Context()); err == nil {
		t.Fatal("Migrate() error = nil, want the schema create refused")
	}
}

// unreachableURL addresses a database no test can reach.
const unreachableURL = "postgres://plugin:plugin@localhost:9/plugin?connect_timeout=1"

// newUnreachablePlugin registers the plugin against a database no test can reach.
func newUnreachablePlugin(t *testing.T, deps sdk.Deps) *Plugin {
	t.Helper()
	deps.DatabaseURL = unreachableURL
	p, err := Register(deps)
	if err != nil {
		t.Fatalf("Register() error = %v, want nil", err)
	}
	t.Cleanup(func() { _ = p.Stop(t.Context()) })
	return p
}

func TestStartReadsNoCatalogue(t *testing.T) {
	t.Parallel()

	p := newUnreachablePlugin(t, sdk.Deps{})

	if err := p.Start(t.Context()); err != nil {
		t.Fatalf("Start() error = %v, want the catalogue left to the first request", err)
	}
}

func TestRegisterHandsTheCatalogueTheHostsBounds(t *testing.T) {
	t.Parallel()

	p := newUnreachablePlugin(t, sdk.Deps{TenantsHeld: 1, TenantsRefresh: time.Hour})
	loader := newFakeLoader()
	p.catalog.loader = loader
	inFirst, first := inTenantOf(t)
	inSecond, _ := inTenantOf(t)

	mustView(t, p.catalog, inFirst)
	mustView(t, p.catalog, inSecond)
	mustView(t, p.catalog, inFirst)

	if got := loader.readsOf(first); got != 2 {
		t.Errorf("first tenant reads = %d, want 2 with one tenant held", got)
	}
	if p.catalog.refresh != time.Hour {
		t.Errorf("refresh = %v, want the host's hour", p.catalog.refresh)
	}
}

func TestFieldsSnapshotServesTheCallersOwnCatalogue(t *testing.T) {
	t.Parallel()

	p := newMigratedPlugin(t)
	acme := inTenant(t, p)
	definedField(t, p, t.Context(), "birthDate")
	definedField(t, p, acme, "shoeSize")

	stamp, held, err := p.FieldsSnapshot(acme)

	if err != nil {
		t.Fatalf("FieldsSnapshot() error = %v, want nil", err)
	}
	if stamp == 0 {
		t.Error("stamp = 0, want the read stamped")
	}
	if len(held) != 1 || held[0].Name != "shoeSize" || held[0].Type != "String" {
		t.Errorf("fields = %+v, want only the caller's shoeSize answering String", held)
	}
}

func TestFieldsSnapshotReportsAnUnreachableCatalogue(t *testing.T) {
	t.Parallel()

	p := newUnreachablePlugin(t, sdk.Deps{})

	if _, _, err := p.FieldsSnapshot(t.Context()); err == nil {
		t.Fatal("FieldsSnapshot() error = nil, want the catalogue read refused")
	}
}

func TestMustSubRejectsInvalidDir(t *testing.T) {
	t.Parallel()

	defer func() {
		if recover() == nil {
			t.Fatal("mustSub(..) did not panic, want a panic")
		}
	}()
	mustSub(migrations, "..")
}
