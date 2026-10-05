// SPDX-License-Identifier: Elastic-2.0

package main

import (
	"testing"

	"github.com/peterldowns/pgtestdb"

	"github.com/gopherium/framework/gonsole"
	"github.com/gopherium/framework/gonsole/testkit"

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

	got := testkit.Run(t, bareProgram(map[string]string{"ALPHONE_DATABASE_URL": databaseURL}), typedPassword+"\n",
		"account:create-admin", "-email", "admin@example.com", "-name", "Admin", "-role", "admin")

	if got.Code != gonsole.ExitDone {
		t.Fatalf("account:create-admin = %d with stderr %q, want 0 on a database holding no schema", got.Code, got.Stderr)
	}
	if held := roleOf(t, databaseURL, "admin@example.com"); held != role.Admin.String() {
		t.Errorf("role = %q, want %q, the first user manages users", held, role.Admin.String())
	}
}
