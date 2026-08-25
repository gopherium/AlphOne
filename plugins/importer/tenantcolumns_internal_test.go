// SPDX-License-Identifier: AGPL-3.0-or-later

package importer

import (
	"testing"
)

// importerScopedTables lists every plugin table that carries the tenant boundary.
var importerScopedTables = []string{"imports", "import_rows"}

func TestEveryImportTableCarriesItsTenant(t *testing.T) {
	t.Parallel()

	pool := newMigratedPool(t)

	for _, table := range importerScopedTables {
		var nullable string
		err := pool.QueryRow(t.Context(),
			"SELECT is_nullable FROM information_schema.columns"+
				" WHERE table_schema = 'plugin_importer' AND table_name = $1"+
				" AND column_name = 'tenant_id'",
			table).Scan(&nullable)
		if err != nil {
			t.Errorf("%s: tenant_id column missing (%v)", table, err)
			continue
		}
		if nullable != "NO" {
			t.Errorf("%s: tenant_id is nullable, want NOT NULL", table)
		}
	}
}
