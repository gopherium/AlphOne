// SPDX-License-Identifier: Elastic-2.0

package email

import (
	"testing"

	"github.com/jackc/pgx/v5"
)

// tenantColumn is one plugin_email table beside whether its tenant_id column allows NULL, missing when it has none.
type tenantColumn struct {
	table    string
	nullable string
}

// tenantColumns returns every plugin_email table but the version table, beside how its tenant_id column takes NULL.
const tenantColumns = `SELECT t.tablename, COALESCE(c.is_nullable::text, 'missing')
	FROM pg_tables t
	LEFT JOIN information_schema.columns c
		ON c.table_schema = t.schemaname AND c.table_name = t.tablename AND c.column_name = 'tenant_id'
	WHERE t.schemaname = 'plugin_email' AND t.tablename <> 'goose_db_version'
	ORDER BY t.tablename`

func TestEveryEmailTableCarriesItsTenant(t *testing.T) {
	t.Parallel()

	pool := newMigratedPool(t)
	rows, _ := pool.Query(t.Context(), tenantColumns)
	columns, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (tenantColumn, error) {
		var column tenantColumn
		return column, row.Scan(&column.table, &column.nullable)
	})
	if err != nil {
		t.Fatalf("reading the plugin_email tables: %v", err)
	}

	if len(columns) == 0 {
		t.Fatal("plugin_email holds no table, want every migrated table read")
	}
	for _, column := range columns {
		if column.nullable != "NO" {
			t.Errorf("plugin_email.%s: tenant_id is_nullable = %s, want NO", column.table, column.nullable)
		}
	}
}
