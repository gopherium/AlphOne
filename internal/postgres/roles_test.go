// SPDX-License-Identifier: Elastic-2.0

package postgres_test

import (
	"context"
	"database/sql"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/peterldowns/pgtestdb"

	"github.com/gopherium/gouncer"

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

// seedSession stores one live session for a user.
func seedSession(t *testing.T, pool *pgxpool.Pool, userID uuid.UUID) {
	t.Helper()
	if _, err := pool.Exec(t.Context(),
		`INSERT INTO auth.sessions (token_hash, user_id, expires_at, created_at)
		VALUES ($1, $2, now() + interval '1 hour', now())`, []byte(uuid.Must(uuid.NewV7()).String()), userID); err != nil {
		t.Fatalf("storing the session: %v", err)
	}
}

// sessionCount returns how many sessions one user holds.
func sessionCount(t *testing.T, pool *pgxpool.Pool, userID uuid.UUID) int {
	t.Helper()
	var held int
	if err := pool.QueryRow(t.Context(),
		"SELECT count(*) FROM auth.sessions WHERE user_id = $1", userID).Scan(&held); err != nil {
		t.Fatalf("counting the sessions: %v", err)
	}
	return held
}

// userDisabled reports whether one user is barred from logging in.
func userDisabled(t *testing.T, pool *pgxpool.Pool, id uuid.UUID) bool {
	t.Helper()
	var disabled bool
	if err := pool.QueryRow(t.Context(),
		"SELECT disabled FROM auth.users WHERE id = $1", id).Scan(&disabled); err != nil {
		t.Fatalf("reading the disabled flag: %v", err)
	}
	return disabled
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

func TestRoleStoreReportsGrantingAUserItCannotFind(t *testing.T) {
	t.Parallel()

	pool := newTestPool(t)
	store := postgres.NewRoleStore(pool)
	ghost := uuid.Must(uuid.NewV7())

	for _, tier := range []role.Role{role.Admin, role.Member} {
		if err := store.Grant(t.Context(), ghost, tier); !errors.Is(err, gouncer.ErrUserNotFound) {
			t.Errorf("Grant(%v) error = %v, want %v", tier, err, gouncer.ErrUserNotFound)
		}
	}
	var rows int
	if err := pool.QueryRow(t.Context(),
		"SELECT count(*) FROM core.user_roles WHERE user_id = $1", ghost).Scan(&rows); err != nil {
		t.Fatalf("counting the rows: %v", err)
	}
	if rows != 0 {
		t.Errorf("stored %d rows for a user nobody holds, want none", rows)
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

func TestRoleStoreRefusesTwoAdminsDemotingEachOtherAtOnce(t *testing.T) {
	t.Parallel()

	pool := newTestPool(t)
	store := postgres.NewRoleStore(pool)
	first := seedPoolUser(t, pool, "first@example.com")
	second := seedPoolUser(t, pool, "second@example.com")
	for _, admin := range []uuid.UUID{first, second} {
		if err := store.Grant(t.Context(), admin, role.Admin); err != nil {
			t.Fatalf("Grant() error = %v, want nil", err)
		}
	}

	holding, err := pool.Begin(t.Context())
	if err != nil {
		t.Fatalf("beginning the holding transaction: %v", err)
	}
	defer func() { _ = holding.Rollback(t.Context()) }()
	if _, err := holding.Exec(t.Context(),
		"SELECT 1 FROM core.user_roles WHERE role = 'admin' FOR UPDATE"); err != nil {
		t.Fatalf("holding the admins still: %v", err)
	}

	demotion := make(chan error, 1)
	go func() { demotion <- store.Grant(context.WithoutCancel(t.Context()), second, role.Member) }()
	select {
	case err := <-demotion:
		t.Fatalf("Grant(member) answered %v while the admins were held, want it waiting its turn", err)
	case <-time.After(250 * time.Millisecond):
	}

	if _, err := holding.Exec(t.Context(),
		"UPDATE core.user_roles SET role = 'member' WHERE user_id = $1", first); err != nil {
		t.Fatalf("demoting the first admin: %v", err)
	}
	if err := holding.Commit(t.Context()); err != nil {
		t.Fatalf("committing the first demotion: %v", err)
	}

	if err := <-demotion; !errors.Is(err, role.ErrLastAdmin) {
		t.Errorf("Grant(member) error = %v, want %v, the second demotion reads the first", err, role.ErrLastAdmin)
	}
	if tier, _ := store.RoleOf(t.Context(), second); tier != role.Admin {
		t.Errorf("the second user stands in %v, want %v left standing", tier, role.Admin)
	}
}

func TestRoleStoreRefusesToDisableTheLastAdmin(t *testing.T) {
	t.Parallel()

	pool := newTestPool(t)
	store := postgres.NewRoleStore(pool)
	only := seedPoolUser(t, pool, "only@example.com")
	if err := store.Grant(t.Context(), only, role.Admin); err != nil {
		t.Fatalf("Grant() error = %v, want nil", err)
	}

	err := store.Disable(t.Context(), only)

	if !errors.Is(err, role.ErrLastAdmin) {
		t.Errorf("Disable() error = %v, want %v", err, role.ErrLastAdmin)
	}
	if disabled := userDisabled(t, pool, only); disabled {
		t.Error("the last admin is disabled, want the deployment left with cover")
	}
}

func TestRoleStoreDisablesAnAdminWhileAnotherStands(t *testing.T) {
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

	if err := store.Disable(t.Context(), leaving); err != nil {
		t.Fatalf("Disable() error = %v, want nil while another admin stands", err)
	}

	if !userDisabled(t, pool, leaving) {
		t.Error("the admin is not disabled, want the change stored")
	}
}

func TestRoleStoreSweepsTheSessionsOfTheUserItDisables(t *testing.T) {
	t.Parallel()

	pool := newTestPool(t)
	store := postgres.NewRoleStore(pool)
	staying := seedPoolUser(t, pool, "staying@example.com")
	leaving := seedPoolUser(t, pool, "leaving@example.com")
	if err := store.Grant(t.Context(), staying, role.Admin); err != nil {
		t.Fatalf("Grant() error = %v, want nil", err)
	}
	seedSession(t, pool, leaving)

	if err := store.Disable(t.Context(), leaving); err != nil {
		t.Fatalf("Disable() error = %v, want nil", err)
	}

	if sessions := sessionCount(t, pool, leaving); sessions != 0 {
		t.Errorf("sessions = %d, want none, a barred user is logged out at once", sessions)
	}
}

func TestRoleStoreDisablesAMemberWhateverTheAdminsAre(t *testing.T) {
	t.Parallel()

	pool := newTestPool(t)
	store := postgres.NewRoleStore(pool)
	only := seedPoolUser(t, pool, "only@example.com")
	staff := seedPoolUser(t, pool, "staff@example.com")
	if err := store.Grant(t.Context(), only, role.Admin); err != nil {
		t.Fatalf("Grant() error = %v, want nil", err)
	}

	if err := store.Disable(t.Context(), staff); err != nil {
		t.Fatalf("Disable() error = %v, want nil, a member is nobody's cover", err)
	}

	if !userDisabled(t, pool, staff) {
		t.Error("the member is not disabled, want the change stored")
	}
}

func TestRoleStoreReportsDisablingAUserItCannotFind(t *testing.T) {
	t.Parallel()

	store := postgres.NewRoleStore(newTestPool(t))

	err := store.Disable(t.Context(), uuid.Must(uuid.NewV7()))

	if !errors.Is(err, gouncer.ErrUserNotFound) {
		t.Errorf("Disable() error = %v, want %v", err, gouncer.ErrUserNotFound)
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
	disabling := store.Disable(t.Context(), uuid.Must(uuid.NewV7()))
	if disabling == nil || errors.Is(disabling, postgres.ErrLastAdmin) {
		t.Errorf("Disable() on a closed pool error = %v, want a connection error", disabling)
	}
}

func TestTheRoleColumnRefusesATierItDoesNotKnow(t *testing.T) {
	t.Parallel()

	pool := newTestPool(t)
	held := seedPoolUser(t, pool, "held@example.com")

	_, err := pool.Exec(t.Context(),
		"INSERT INTO core.user_roles (user_id, role) VALUES ($1, 'root')", held)

	if err == nil {
		t.Error("an unknown tier was stored, want the column to refuse it")
	}
}

func TestTheRoleTableRefusesAUserNobodyHolds(t *testing.T) {
	t.Parallel()

	pool := newTestPool(t)

	_, err := pool.Exec(t.Context(),
		"INSERT INTO core.user_roles (user_id, role) VALUES ($1, 'member')", uuid.Must(uuid.NewV7()))

	if err == nil {
		t.Error("a tier was stored for a user nobody holds, want the table to refuse it")
	}
}
