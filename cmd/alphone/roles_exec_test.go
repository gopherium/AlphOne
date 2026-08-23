// SPDX-License-Identifier: Elastic-2.0

package main

import (
	"database/sql"
	"strings"
	"testing"

	"github.com/peterldowns/pgtestdb"

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

func TestCreateAdminProvisionsAnAdminOnABareDatabase(t *testing.T) {
	t.Parallel()

	databaseURL := barePostgres(t)
	getenv := testGetenv(map[string]string{"ALPHONE_DATABASE_URL": databaseURL})

	err := createAdmin(t.Context(), getenv,
		[]string{"-email", "admin@example.com", "-name", "Admin", "-role", "admin"},
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
		"SELECT role FROM auth.users WHERE email = 'admin@example.com'").Scan(&tier); err != nil {
		t.Fatalf("reading the provisioned role: %v", err)
	}
	if tier != role.Admin.String() {
		t.Errorf("role = %q, want %q, the first user manages users", tier, role.Admin.String())
	}
}
