// SPDX-License-Identifier: Elastic-2.0

package main

import (
	"database/sql"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/peterldowns/pgtestdb"

	authkitpg "github.com/gopherium/gouncer/authkit/postgres"

	"github.com/gopherium/alphone/internal/role"
	"github.com/gopherium/alphone/internal/testdb"
)

// barePostgres returns a database holding neither the auth schema nor the core schema.
func barePostgres(t *testing.T) string {
	t.Helper()
	if testing.Short() {
		t.Skip("skipping database test in short mode")
	}
	return pgtestdb.Custom(t, testdb.Config(), pgtestdb.NoopMigrator{}).URL()
}

func TestGrantAdminReportsAnUnknownUser(t *testing.T) {
	t.Parallel()

	pool, err := pgxpool.New(t.Context(), testDatabaseURL(t))
	if err != nil {
		t.Fatalf("connecting pool: %v", err)
	}
	t.Cleanup(pool.Close)

	err = grantAdmin(t.Context(), pool, authkitpg.NewUserStore(pool), "nobody@example.com")

	if err == nil {
		t.Error("grantAdmin() error = nil, want a refusal for a user that does not exist")
	}
}

func TestCreateAdminProvisionsAnAdminOnABareDatabase(t *testing.T) {
	t.Parallel()

	databaseURL := barePostgres(t)
	getenv := testGetenv(map[string]string{"ALPHONE_DATABASE_URL": databaseURL})

	err := createAdmin(t.Context(), getenv,
		[]string{"-email", "admin@example.com", "-name", "Admin"},
		strings.NewReader("correct horse battery\n"), &strings.Builder{})

	if err != nil {
		t.Fatalf("createAdmin() error = %v, want nil on a database holding no schema", err)
	}
	db, err := sql.Open("pgx", databaseURL)
	if err != nil {
		t.Fatalf("opening the database: %v", err)
	}
	defer func() { _ = db.Close() }()
	var tier string
	if err := db.QueryRowContext(t.Context(),
		`SELECT r.role FROM core.user_roles r
		JOIN auth.users u ON u.id = r.user_id WHERE u.email = 'admin@example.com'`).Scan(&tier); err != nil {
		t.Fatalf("reading the provisioned role: %v", err)
	}
	if tier != role.Admin.String() {
		t.Errorf("role = %q, want %q, the first user manages users", tier, role.Admin.String())
	}
}
