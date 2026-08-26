// SPDX-License-Identifier: Elastic-2.0

package fields

import (
	"testing"

	"github.com/google/uuid"
)

// fieldsScopedTables lists every plugin table that carries the tenant boundary.
var fieldsScopedTables = []string{"definitions", "contact_values"}

func TestEveryFieldsTableCarriesItsTenant(t *testing.T) {
	t.Parallel()

	p := newMigratedPlugin(t)

	for _, table := range fieldsScopedTables {
		var nullable string
		err := p.pool.QueryRow(t.Context(),
			"SELECT is_nullable FROM information_schema.columns"+
				" WHERE table_schema = 'plugin_fields' AND table_name = $1"+
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

func TestTwoTenantsMayDefineTheSameFieldName(t *testing.T) {
	t.Parallel()

	p := newMigratedPlugin(t)
	acme := uuid.Must(uuid.NewV7())
	if _, err := p.pool.Exec(t.Context(),
		"INSERT INTO core.tenants (id, name) VALUES ($1, $2)", acme, "Acme"); err != nil {
		t.Fatalf("seeding the tenant: %v", err)
	}
	if _, err := p.pool.Exec(t.Context(),
		"INSERT INTO plugin_fields.definitions (id, name, label, kind, created_at)"+
			" VALUES ($1, 'birthday', 'Birthday', 'DATE', now())",
		uuid.Must(uuid.NewV7())); err != nil {
		t.Fatalf("the default tenant definition: %v", err)
	}

	_, err := p.pool.Exec(t.Context(),
		"INSERT INTO plugin_fields.definitions (id, name, label, kind, created_at, tenant_id)"+
			" VALUES ($1, 'birthday', 'Birthday', 'DATE', now(), $2)",
		uuid.Must(uuid.NewV7()), acme)

	if err != nil {
		t.Errorf("the same field name under another tenant: %v, want it admitted", err)
	}
}

func TestOneTenantRefusesADuplicateFieldName(t *testing.T) {
	t.Parallel()

	p := newMigratedPlugin(t)
	if _, err := p.pool.Exec(t.Context(),
		"INSERT INTO plugin_fields.definitions (id, name, label, kind, created_at)"+
			" VALUES ($1, 'birthday', 'Birthday', 'DATE', now())",
		uuid.Must(uuid.NewV7())); err != nil {
		t.Fatalf("the first definition: %v", err)
	}

	_, err := p.pool.Exec(t.Context(),
		"INSERT INTO plugin_fields.definitions (id, name, label, kind, created_at)"+
			" VALUES ($1, 'birthday', 'Twice', 'DATE', now())",
		uuid.Must(uuid.NewV7()))

	if err == nil {
		t.Error("a duplicate field name in one tenant was stored, want the composite to refuse it")
	}
}
