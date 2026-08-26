// SPDX-License-Identifier: Elastic-2.0

package postgres_test

import (
	"database/sql"
	"io/fs"
	"testing"

	"github.com/google/uuid"
	"github.com/peterldowns/pgtestdb"
	"github.com/pressly/goose/v3"
	"github.com/pressly/goose/v3/database"

	authkitpg "github.com/gopherium/gouncer/authkit/postgres"

	"github.com/gopherium/alphone/internal/postgres"
	"github.com/gopherium/alphone/internal/tenant"
	"github.com/gopherium/alphone/internal/testdb"
)

func TestTenantForUserAnswersTheDefaultWithoutARow(t *testing.T) {
	t.Parallel()

	store := postgres.NewTenantStore(newTestPool(t))

	got, err := store.TenantForUser(t.Context(), uuid.Must(uuid.NewV7()))

	if err != nil {
		t.Fatalf("TenantForUser() error = %v, want nil", err)
	}
	if got.ID != tenant.DefaultID {
		t.Errorf("id = %s, want the default tenant %s", got.ID, tenant.DefaultID)
	}
	if got.Name != "Default" {
		t.Errorf("name = %q, want Default", got.Name)
	}
}

func TestTenantForUserAnswersThePlacedTenant(t *testing.T) {
	t.Parallel()

	pool := newTestPool(t)
	store := postgres.NewTenantStore(pool)
	acme := uuid.Must(uuid.NewV7())
	member := uuid.Must(uuid.NewV7())
	if _, err := pool.Exec(t.Context(),
		"INSERT INTO core.tenants (id, name) VALUES ($1, $2)", acme, "Acme"); err != nil {
		t.Fatalf("seeding the tenant: %v", err)
	}
	if _, err := pool.Exec(t.Context(),
		"INSERT INTO core.tenant_members (user_id, tenant_id) VALUES ($1, $2)", member, acme); err != nil {
		t.Fatalf("placing the member: %v", err)
	}

	got, err := store.TenantForUser(t.Context(), member)

	if err != nil {
		t.Fatalf("TenantForUser() error = %v, want nil", err)
	}
	if got.ID != acme || got.Name != "Acme" {
		t.Errorf("tenant = %+v, want the placed Acme tenant", got)
	}
}

func TestTenantForUserReportsADeactivatedTenant(t *testing.T) {
	t.Parallel()

	pool := newTestPool(t)
	store := postgres.NewTenantStore(pool)
	acme := uuid.Must(uuid.NewV7())
	member := uuid.Must(uuid.NewV7())
	if _, err := pool.Exec(t.Context(),
		"INSERT INTO core.tenants (id, name, deactivated_at) VALUES ($1, $2, now())", acme, "Acme"); err != nil {
		t.Fatalf("seeding the deactivated tenant: %v", err)
	}
	if _, err := pool.Exec(t.Context(),
		"INSERT INTO core.tenant_members (user_id, tenant_id) VALUES ($1, $2)", member, acme); err != nil {
		t.Fatalf("placing the member: %v", err)
	}

	got, err := store.TenantForUser(t.Context(), member)

	if err != nil {
		t.Fatalf("TenantForUser() error = %v, want nil", err)
	}
	if !got.Deactivated {
		t.Error("Deactivated = false, want the deactivation reported")
	}
}

func TestTenantForUserReportsALiveTenantAsActive(t *testing.T) {
	t.Parallel()

	store := postgres.NewTenantStore(newTestPool(t))

	got, err := store.TenantForUser(t.Context(), uuid.Must(uuid.NewV7()))

	if err != nil {
		t.Fatalf("TenantForUser() error = %v, want nil", err)
	}
	if got.Deactivated {
		t.Error("Deactivated = true for the default tenant, want it active")
	}
}

func TestTenantsOfAnswersOnlyTheUsersARowPlaces(t *testing.T) {
	t.Parallel()

	pool := newTestPool(t)
	store := postgres.NewTenantStore(pool)
	acme := uuid.Must(uuid.NewV7())
	member := uuid.Must(uuid.NewV7())
	unplaced := uuid.Must(uuid.NewV7())
	if _, err := pool.Exec(t.Context(),
		"INSERT INTO core.tenants (id, name) VALUES ($1, $2)", acme, "Acme"); err != nil {
		t.Fatalf("seeding the tenant: %v", err)
	}
	if _, err := pool.Exec(t.Context(),
		"INSERT INTO core.tenant_members (user_id, tenant_id) VALUES ($1, $2)", member, acme); err != nil {
		t.Fatalf("placing the member: %v", err)
	}

	placed, err := store.TenantsOf(t.Context(), []uuid.UUID{member, unplaced})

	if err != nil {
		t.Fatalf("TenantsOf() error = %v, want nil", err)
	}
	if placed[member] != acme {
		t.Errorf("placed[member] = %v, want %v", placed[member], acme)
	}
	if _, held := placed[unplaced]; held {
		t.Error("placed carried the unplaced account, want absence answering the default")
	}
}

func TestTenantsOfReportsAStoreFailure(t *testing.T) {
	t.Parallel()

	pool := newTestPool(t)
	store := postgres.NewTenantStore(pool)
	pool.Close()

	if _, err := store.TenantsOf(t.Context(), []uuid.UUID{uuid.Must(uuid.NewV7())}); err == nil {
		t.Error("TenantsOf() error = nil, want the closed pool reported")
	}
}

func TestMigrationSeedsTheDefaultTenantExactlyOnce(t *testing.T) {
	t.Parallel()

	pool := newTestPool(t)

	var held int
	if err := pool.QueryRow(t.Context(),
		"SELECT count(*) FROM core.tenants WHERE id = $1", tenant.DefaultID).Scan(&held); err != nil {
		t.Fatalf("reading the default tenant: %v", err)
	}

	if held != 1 {
		t.Errorf("the store holds %d default tenants, want exactly 1", held)
	}
}

func TestTenantsMigrationUpgradesAnExistingDatabase(t *testing.T) {
	t.Parallel()
	if testing.Short() {
		t.Skip("skipping database test in short mode")
	}

	cfg := pgtestdb.Custom(t, testdb.Config(), pgtestdb.NoopMigrator{})
	if err := authkitpg.Migrate(t.Context(), cfg.URL()); err != nil {
		t.Fatalf("migrating the auth schema: %v", err)
	}
	db, err := sql.Open("pgx", cfg.URL())
	if err != nil {
		t.Fatalf("opening the database: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	provider := newCoreProvider(t, db)
	if _, err := provider.UpTo(t.Context(), 10); err != nil {
		t.Fatalf("migrating to the previous version: %v", err)
	}
	var missing *string
	if err := db.QueryRowContext(t.Context(),
		"SELECT to_regclass('core.tenants')::text").Scan(&missing); err != nil {
		t.Fatalf("probing the relation: %v", err)
	}
	if missing != nil {
		t.Fatal("core.tenants exists before its migration, the version pin is wrong")
	}

	if _, err := provider.Up(t.Context()); err != nil {
		t.Fatalf("migrating forward: %v", err)
	}

	var name string
	if err := db.QueryRowContext(t.Context(),
		"SELECT name FROM core.tenants WHERE id = $1", tenant.DefaultID).Scan(&name); err != nil {
		t.Fatalf("reading the default tenant: %v", err)
	}
	if name != "Default" {
		t.Errorf("name = %q, want Default on an upgraded database", name)
	}
}

// newCoreProvider builds a goose provider over the embedded core migrations.
func newCoreProvider(t *testing.T, db *sql.DB) *goose.Provider {
	t.Helper()
	store, err := database.NewStore(database.DialectPostgres, "goose_db_version")
	if err != nil {
		t.Fatalf("building the migration store: %v", err)
	}
	source, err := fs.Sub(postgres.Migrations, "migrations")
	if err != nil {
		t.Fatalf("opening the migration source: %v", err)
	}
	provider, err := goose.NewProvider("", db, source, goose.WithStore(store))
	if err != nil {
		t.Fatalf("building the migration provider: %v", err)
	}
	return provider
}

func TestTenantForUserReportsAStoreFailure(t *testing.T) {
	t.Parallel()

	pool := newTestPool(t)
	store := postgres.NewTenantStore(pool)
	pool.Close()

	if _, err := store.TenantForUser(t.Context(), uuid.Must(uuid.NewV7())); err == nil {
		t.Error("TenantForUser() error = nil, want the closed pool reported")
	}
}
