// SPDX-License-Identifier: AGPL-3.0-or-later

package postgres

import (
	"testing"
)

func TestMigrateRequiresDatabase(t *testing.T) {
	t.Parallel()

	if err := migrate(t.Context(), nil); err == nil {
		t.Fatal("migrate(nil) error = nil, want a provider error")
	}
}

func TestMigrateDatabaseRequiresRegisteredDriver(t *testing.T) {
	t.Parallel()

	if err := migrateDatabase(t.Context(), "no-such-driver", "ignored"); err == nil {
		t.Fatal("migrateDatabase() error = nil, want an unknown driver error")
	}
}

func TestMustSubRejectsInvalidDir(t *testing.T) {
	t.Parallel()

	defer func() {
		if recover() == nil {
			t.Fatal("mustSub(..) did not panic, want a panic")
		}
	}()

	mustSub(Migrations, "..")
}
