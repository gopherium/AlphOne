// SPDX-License-Identifier: Elastic-2.0

package fields

import (
	"database/sql"
	"testing"

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

func TestStartReportsAnUnreachableCatalogue(t *testing.T) {
	t.Parallel()

	p, err := Register(sdk.Deps{DatabaseURL: "postgres://plugin:plugin@localhost:9/plugin?connect_timeout=1"})
	if err != nil {
		t.Fatalf("Register() error = %v, want nil", err)
	}
	t.Cleanup(func() { _ = p.Stop(t.Context()) })

	if err := p.Start(t.Context()); err == nil {
		t.Fatal("Start() error = nil, want the catalogue read refused")
	}
}

func TestFieldsSnapshotServesTheLoadedCatalogue(t *testing.T) {
	t.Parallel()

	p := newMigratedPlugin(t)
	if err := p.store.create(t.Context(), defined(t, "birthDate", "DATE")); err != nil {
		t.Fatalf("create() error = %v, want nil", err)
	}
	if err := p.catalog.reload(t.Context()); err != nil {
		t.Fatalf("reload() error = %v, want nil", err)
	}

	version, held, err := p.FieldsSnapshot(t.Context())

	if err != nil {
		t.Fatalf("FieldsSnapshot() error = %v, want nil", err)
	}
	if version == 0 {
		t.Error("version = 0, want the loaded catalogue's version")
	}
	if len(held) != 1 || held[0].Name != "birthDate" || held[0].Type != "Date" {
		t.Errorf("fields = %+v, want birthDate answering Date", held)
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
