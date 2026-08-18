// SPDX-License-Identifier: Elastic-2.0

package postgres_test

import (
	"database/sql"
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/peterldowns/pgtestdb"

	"github.com/gopherium/alphone/internal/postgres"
	"github.com/gopherium/alphone/internal/role"
	"github.com/gopherium/alphone/internal/testdb"
)

// seedPoolUser stores one auth user through the pool and returns its id.
func seedPoolUser(t *testing.T, pool *pgxpool.Pool, email string) uuid.UUID {
	t.Helper()
	id := uuid.Must(uuid.NewV7())
	if _, err := pool.Exec(t.Context(),
		`INSERT INTO auth.users (id, email, name, password_hash, disabled, created_at)
		VALUES ($1, $2, 'Maria Perez', 'hash', false, now())`, id, email); err != nil {
		t.Fatalf("storing the user: %v", err)
	}
	return id
}

// seedUser stores one auth user and returns its id.
func seedUser(t *testing.T, db *sql.DB, email string) uuid.UUID {
	t.Helper()
	id := uuid.Must(uuid.NewV7())
	if _, err := db.ExecContext(t.Context(),
		`INSERT INTO auth.users (id, email, name, password_hash, disabled, created_at)
		VALUES ($1, $2, 'Maria Perez', 'hash', false, now())`, id, email); err != nil {
		t.Fatalf("storing the user: %v", err)
	}
	return id
}

// roleOf returns the tier stored for one user.
func roleOf(t *testing.T, db *sql.DB, id uuid.UUID) string {
	t.Helper()
	var stored string
	err := db.QueryRowContext(t.Context(), "SELECT role FROM core.user_roles WHERE user_id = $1", id).Scan(&stored)
	if err == sql.ErrNoRows {
		return ""
	}
	if err != nil {
		t.Fatalf("reading the role: %v", err)
	}
	return stored
}

func TestMigrationGrantsAdminToAUserFromBeforeRoles(t *testing.T) {
	t.Parallel()
	if testing.Short() {
		t.Skip("skipping database test in short mode")
	}

	cfg := pgtestdb.Custom(t, testdb.Config(), testdb.Migrator())
	db, err := sql.Open("pgx", cfg.URL())
	if err != nil {
		t.Fatalf("opening the database: %v", err)
	}
	defer func() { _ = db.Close() }()
	provider := coreProvider(t, db)
	if _, err := provider.DownTo(t.Context(), grantedRolesVersion-1); err != nil {
		t.Fatalf("rolling back to the schema before roles: %v", err)
	}
	existing := seedUser(t, db, "before@example.com")

	if _, err := provider.UpTo(t.Context(), grantedRolesVersion); err != nil {
		t.Fatalf("applying the roles migration: %v", err)
	}

	if got := roleOf(t, db, existing); got != role.Admin.String() {
		t.Errorf("role = %q, want %q, a user from before roles keeps the authority it had",
			got, role.Admin.String())
	}
}

func TestAUserCreatedAfterTheMigrationHoldsNoRow(t *testing.T) {
	t.Parallel()
	if testing.Short() {
		t.Skip("skipping database test in short mode")
	}

	cfg := pgtestdb.Custom(t, testdb.Config(), testdb.Migrator())
	db, err := sql.Open("pgx", cfg.URL())
	if err != nil {
		t.Fatalf("opening the database: %v", err)
	}
	defer func() { _ = db.Close() }()

	fresh := seedUser(t, db, "after@example.com")

	if got := roleOf(t, db, fresh); got != "" {
		t.Errorf("role = %q, want no row, absence is what makes a member", got)
	}
}

func TestRoleStoreReadsMemberForAUserItHoldsNoRowFor(t *testing.T) {
	t.Parallel()

	store := postgres.NewRoleStore(newTestPool(t))

	got, err := store.RoleOf(t.Context(), uuid.Must(uuid.NewV7()))

	if err != nil {
		t.Fatalf("RoleOf() error = %v, want nil", err)
	}
	if got != role.Member {
		t.Errorf("RoleOf() = %v, want %v, absence is what makes a member", got, role.Member)
	}
}

func TestRoleStoreRoundTripsATier(t *testing.T) {
	t.Parallel()

	pool := newTestPool(t)
	store := postgres.NewRoleStore(pool)
	owner := seedPoolUser(t, pool, "owner@example.com")

	if err := store.Grant(t.Context(), owner, role.Admin); err != nil {
		t.Fatalf("Grant() error = %v, want nil", err)
	}

	got, err := store.RoleOf(t.Context(), owner)
	if err != nil {
		t.Fatalf("RoleOf() error = %v, want nil", err)
	}
	if got != role.Admin {
		t.Errorf("RoleOf() = %v, want %v", got, role.Admin)
	}
}

func TestRoleStoreRefusesToDemoteTheLastAdmin(t *testing.T) {
	t.Parallel()

	pool := newTestPool(t)
	store := postgres.NewRoleStore(pool)
	only := seedPoolUser(t, pool, "only@example.com")
	if err := store.Grant(t.Context(), only, role.Admin); err != nil {
		t.Fatalf("Grant() error = %v, want nil", err)
	}

	err := store.Grant(t.Context(), only, role.Member)

	if !errors.Is(err, postgres.ErrLastAdmin) {
		t.Errorf("Grant(member) error = %v, want %v", err, postgres.ErrLastAdmin)
	}
	got, _ := store.RoleOf(t.Context(), only)
	if got != role.Admin {
		t.Errorf("RoleOf() = %v, want the admin left standing", got)
	}
}

func TestRoleStoreCountsOnlyEnabledAdminsAsCover(t *testing.T) {
	t.Parallel()

	pool := newTestPool(t)
	store := postgres.NewRoleStore(pool)
	staying := seedPoolUser(t, pool, "staying@example.com")
	leaving := seedPoolUser(t, pool, "leaving@example.com")
	for _, admin := range []uuid.UUID{staying, leaving} {
		if err := store.Grant(t.Context(), admin, role.Admin); err != nil {
			t.Fatalf("Grant() error = %v, want nil", err)
		}
	}
	if _, err := pool.Exec(t.Context(),
		"UPDATE auth.users SET disabled = true WHERE id = $1", leaving); err != nil {
		t.Fatalf("disabling the other admin: %v", err)
	}

	err := store.Grant(t.Context(), staying, role.Member)

	if !errors.Is(err, postgres.ErrLastAdmin) {
		t.Errorf("Grant(member) error = %v, want %v, a disabled admin is dead cover", err, postgres.ErrLastAdmin)
	}
}

func TestRoleStoreDemotesAnAdminWhileAnotherStands(t *testing.T) {
	t.Parallel()

	pool := newTestPool(t)
	store := postgres.NewRoleStore(pool)
	staying := seedPoolUser(t, pool, "staying@example.com")
	leaving := seedPoolUser(t, pool, "leaving@example.com")
	for _, admin := range []uuid.UUID{staying, leaving} {
		if err := store.Grant(t.Context(), admin, role.Admin); err != nil {
			t.Fatalf("Grant() error = %v, want nil", err)
		}
	}

	if err := store.Grant(t.Context(), leaving, role.Member); err != nil {
		t.Fatalf("Grant(member) error = %v, want nil while another admin stands", err)
	}

	got, _ := store.RoleOf(t.Context(), leaving)
	if got != role.Member {
		t.Errorf("RoleOf() = %v, want %v", got, role.Member)
	}
}

func TestRoleStoreReadsManyTiersAtOnce(t *testing.T) {
	t.Parallel()

	pool := newTestPool(t)
	store := postgres.NewRoleStore(pool)
	boss := seedPoolUser(t, pool, "boss@example.com")
	staff := seedPoolUser(t, pool, "staff@example.com")
	if err := store.Grant(t.Context(), boss, role.Admin); err != nil {
		t.Fatalf("Grant() error = %v, want nil", err)
	}

	tiers, err := store.RolesOf(t.Context(), []uuid.UUID{boss, staff})

	if err != nil {
		t.Fatalf("RolesOf() error = %v, want nil", err)
	}
	if tiers[boss] != role.Admin {
		t.Errorf("boss = %v, want %v", tiers[boss], role.Admin)
	}
	if tiers[staff] != role.Member {
		t.Errorf("staff = %v, want %v, a user with no row is a member", tiers[staff], role.Member)
	}
}

func TestRoleStoreReadsNoTiersForNobody(t *testing.T) {
	t.Parallel()

	store := postgres.NewRoleStore(newTestPool(t))

	tiers, err := store.RolesOf(t.Context(), nil)

	if err != nil {
		t.Fatalf("RolesOf() error = %v, want nil", err)
	}
	if len(tiers) != 0 {
		t.Errorf("RolesOf(nobody) = %v, want empty", tiers)
	}
}

func TestRoleStoreReportsAConnectionFailure(t *testing.T) {
	t.Parallel()

	pool := newTestPool(t)
	store := postgres.NewRoleStore(pool)
	pool.Close()

	if _, err := store.RoleOf(t.Context(), uuid.Must(uuid.NewV7())); err == nil {
		t.Error("RoleOf() on a closed pool error = nil, want error")
	}
	if _, err := store.RolesOf(t.Context(), []uuid.UUID{uuid.Must(uuid.NewV7())}); err == nil {
		t.Error("RolesOf() on a closed pool error = nil, want error")
	}
	if err := store.Grant(t.Context(), uuid.Must(uuid.NewV7()), role.Admin); err == nil {
		t.Error("Grant(admin) on a closed pool error = nil, want error")
	}
	err := store.Grant(t.Context(), uuid.Must(uuid.NewV7()), role.Member)
	if err == nil || errors.Is(err, postgres.ErrLastAdmin) {
		t.Errorf("Grant(member) on a closed pool error = %v, want a connection error", err)
	}
}

func TestTheRoleColumnRefusesATierItDoesNotKnow(t *testing.T) {
	t.Parallel()

	pool := newTestPool(t)

	_, err := pool.Exec(t.Context(),
		"INSERT INTO core.user_roles (user_id, role) VALUES ($1, 'root')", uuid.Must(uuid.NewV7()))

	if err == nil {
		t.Error("an unknown tier was stored, want the column to refuse it")
	}
}
