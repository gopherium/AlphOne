// SPDX-License-Identifier: Elastic-2.0

package main

import (
	"context"
	"errors"
	"testing"

	"github.com/pressly/goose/v3/lock"

	authkitpg "github.com/gopherium/gouncer/authkit/postgres"
)

// accountsTable is the table the first account migration creates.
const accountsTable = "auth.users"

func TestTheAccountMigrationWaitsForTheMigrationLock(t *testing.T) {
	t.Parallel()

	address := barePostgres(t)
	rival := sessionTo(t, address)
	if _, err := rival.Exec(t.Context(), "SELECT pg_advisory_lock($1)", lock.DefaultLockID); err != nil {
		t.Fatalf("holding the migration lock: %v", err)
	}
	ctx, cancel := context.WithTimeout(t.Context(), lockedMigrateBudget)
	defer cancel()

	err := authkitpg.Migrate(ctx, address)

	var created bool
	if err := rival.QueryRow(t.Context(), "SELECT to_regclass($1) IS NOT NULL", accountsTable).Scan(&created); err != nil {
		t.Fatalf("asking for %s: %v", accountsTable, err)
	}
	if !errors.Is(err, context.DeadlineExceeded) || created {
		t.Errorf("the account migration = %v with %s created %v, want the deadline and no %s table",
			err, accountsTable, created, accountsTable)
	}
}
