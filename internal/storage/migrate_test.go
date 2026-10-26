// SPDX-License-Identifier: Elastic-2.0

package storage_test

import (
	"strings"
	"testing"

	"github.com/gopherium/alphone/internal/storage"
)

// storageTables counts the two tables of the storage schema.
const storageTables = "SELECT count(*) FROM pg_tables " +
	"WHERE schemaname = 'storage' AND tablename IN ('objects', 'chunks')"

func TestMigrateCreatesTheStorageTablesOnce(t *testing.T) {
	t.Parallel()

	address := bareDatabase(t)

	for run := range 2 {
		if err := storage.Migrate(t.Context(), address); err != nil {
			t.Fatalf("Migrate() run %d error = %v, want nil", run+1, err)
		}
	}

	if tables := count(t, poolOn(t, address), storageTables); tables != 2 {
		t.Errorf("the storage schema holds %d of its tables, want both", tables)
	}
}

func TestMigrateNamesTheSchemaItCouldNotCreate(t *testing.T) {
	t.Parallel()

	err := storage.Migrate(t.Context(),
		"postgres://postgres:alphone@localhost:9/postgres?sslmode=disable&connect_timeout=1")

	if err == nil || !strings.Contains(err.Error(), "migrate the schema storage") {
		t.Fatalf("Migrate() on a server nobody runs error = %v, want the storage schema named", err)
	}
}

func TestMigrateNamesAnAddressItCannotRead(t *testing.T) {
	t.Parallel()

	err := storage.Migrate(t.Context(), "postgres://localhost:5433/alphone?sslmode=sometimes")

	if err == nil || !strings.HasPrefix(err.Error(), "storage: read the database address: ") {
		t.Fatalf("Migrate() error = %v, want the address refused before any connection", err)
	}
}
