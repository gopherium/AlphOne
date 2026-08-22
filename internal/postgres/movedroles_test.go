// SPDX-License-Identifier: Elastic-2.0

package postgres_test

import (
	"database/sql"
	"testing"

	"github.com/google/uuid"
	"github.com/peterldowns/pgtestdb"

	"github.com/gopherium/alphone/internal/role"
	"github.com/gopherium/alphone/internal/testdb"
)

// seedUser stores one user and returns its identifier.
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

// storedRole returns the role the brick's column holds for a user.
func storedRole(t *testing.T, db *sql.DB, userID uuid.UUID) string {
	t.Helper()
	var held string
	if err := db.QueryRowContext(t.Context(),
		"SELECT role FROM auth.users WHERE id = $1", userID).Scan(&held); err != nil {
		t.Fatalf("reading the role: %v", err)
	}
	return held
}

// grantTier stores one core.user_roles row for a user.
func grantTier(t *testing.T, db *sql.DB, userID uuid.UUID, tier string) {
	t.Helper()
	if _, err := db.ExecContext(t.Context(),
		"INSERT INTO core.user_roles (user_id, role) VALUES ($1, $2)", userID, tier); err != nil {
		t.Fatalf("granting %q: %v", tier, err)
	}
}

func TestMigrationMovesEveryTierOntoTheAccount(t *testing.T) {
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
	if _, err := provider.DownTo(t.Context(), movedRolesVersion-1); err != nil {
		t.Fatalf("rolling back to the schema before the move: %v", err)
	}
	standing := seedUser(t, db, "admin@example.com")
	working := seedUser(t, db, "member@example.com")
	absent := seedUser(t, db, "norow@example.com")
	grantTier(t, db, standing, role.Admin.String())
	grantTier(t, db, working, role.Member.String())

	if _, err := provider.UpTo(t.Context(), movedRolesVersion); err != nil {
		t.Fatalf("applying the move: %v", err)
	}

	if got := storedRole(t, db, standing); got != role.Admin.String() {
		t.Errorf("role = %q, want %q, an admin keeps the authority it had", got, role.Admin.String())
	}
	if got := storedRole(t, db, working); got != role.Member.String() {
		t.Errorf("role = %q, want %q", got, role.Member.String())
	}
	if got := storedRole(t, db, absent); got != role.Member.String() {
		t.Errorf("role = %q, want %q, an account holding no row is a member", got, role.Member.String())
	}
}

func TestMigrationLeavesNoRolesTableBehind(t *testing.T) {
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

	var held bool
	if err := db.QueryRowContext(t.Context(),
		`SELECT EXISTS (
			SELECT 1 FROM information_schema.tables
			WHERE table_schema = 'core' AND table_name = 'user_roles'
		)`).Scan(&held); err != nil {
		t.Fatalf("reading the table list: %v", err)
	}

	if held {
		t.Error("core.user_roles still stands, want the move to drop it")
	}
}

func TestMigrationRestoresTheRolesTableGoingDown(t *testing.T) {
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
	standing := seedUser(t, db, "admin@example.com")
	if _, err := db.ExecContext(t.Context(),
		"UPDATE auth.users SET role = $1 WHERE id = $2", role.Admin.String(), standing); err != nil {
		t.Fatalf("standing the user in the admin tier: %v", err)
	}

	if _, err := coreProvider(t, db).DownTo(t.Context(), movedRolesVersion-1); err != nil {
		t.Fatalf("rolling the move back: %v", err)
	}

	var held string
	if err := db.QueryRowContext(t.Context(),
		"SELECT role FROM core.user_roles WHERE user_id = $1", standing).Scan(&held); err != nil {
		t.Fatalf("reading the restored row: %v", err)
	}
	if held != role.Admin.String() {
		t.Errorf("restored role = %q, want %q", held, role.Admin.String())
	}
}
