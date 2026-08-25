// SPDX-License-Identifier: Elastic-2.0

package postgres_test

import (
	"testing"

	"github.com/google/uuid"

	"github.com/gopherium/alphone/internal/tenant"
)

// coreScopedTables lists every core table that carries the tenant boundary.
var coreScopedTables = []string{
	"contacts",
	"contact_identities",
	"tasks",
	"api_tokens",
	"webhook_subscriptions",
	"webhook_deliveries",
	"user_settings",
}

func TestEveryCoreDataTableCarriesItsTenant(t *testing.T) {
	t.Parallel()

	pool := newTestPool(t)

	for _, table := range coreScopedTables {
		var nullable string
		err := pool.QueryRow(t.Context(),
			"SELECT is_nullable FROM information_schema.columns"+
				" WHERE table_schema = 'core' AND table_name = $1 AND column_name = 'tenant_id'",
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

func TestAnExistingRowStandsInTheDefaultTenant(t *testing.T) {
	t.Parallel()

	pool := newTestPool(t)
	contactID := uuid.Must(uuid.NewV7())
	if _, err := pool.Exec(t.Context(),
		"INSERT INTO core.contacts (id, name, created_at) VALUES ($1, $2, now())",
		contactID, "Maria Perez"); err != nil {
		t.Fatalf("inserting without a tenant: %v", err)
	}

	var standing uuid.UUID
	err := pool.QueryRow(t.Context(),
		"SELECT tenant_id FROM core.contacts WHERE id = $1", contactID).Scan(&standing)

	if err != nil {
		t.Fatalf("reading the tenant: %v", err)
	}
	if standing != tenant.DefaultID {
		t.Errorf("tenant_id = %v, want the default tenant", standing)
	}
}

func TestTwoTenantsMayHoldTheSameContactIdentity(t *testing.T) {
	t.Parallel()

	pool := newTestPool(t)
	acme := uuid.Must(uuid.NewV7())
	if _, err := pool.Exec(t.Context(),
		"INSERT INTO core.tenants (id, name) VALUES ($1, $2)", acme, "Acme"); err != nil {
		t.Fatalf("seeding the tenant: %v", err)
	}
	first, second := uuid.Must(uuid.NewV7()), uuid.Must(uuid.NewV7())
	if _, err := pool.Exec(t.Context(),
		"INSERT INTO core.contacts (id, name, created_at) VALUES ($1, $2, now()), ($3, $4, now())",
		first, "Maria Perez", second, "Ada Lovelace"); err != nil {
		t.Fatalf("seeding the contacts: %v", err)
	}
	if _, err := pool.Exec(t.Context(),
		"INSERT INTO core.contact_identities (id, contact_id, channel, identifier, display_name, created_at)"+
			" VALUES ($1, $2, 'phone', '184467235', '', now())",
		uuid.Must(uuid.NewV7()), first); err != nil {
		t.Fatalf("the default tenant identity: %v", err)
	}

	_, err := pool.Exec(t.Context(),
		"INSERT INTO core.contact_identities"+
			" (id, contact_id, channel, identifier, display_name, created_at, tenant_id)"+
			" VALUES ($1, $2, 'phone', '184467235', '', now(), $3)",
		uuid.Must(uuid.NewV7()), second, acme)

	if err != nil {
		t.Errorf("the same identity under another tenant: %v, want it admitted", err)
	}
}

func TestOneTenantRefusesADuplicateContactIdentity(t *testing.T) {
	t.Parallel()

	pool := newTestPool(t)
	contactID := uuid.Must(uuid.NewV7())
	if _, err := pool.Exec(t.Context(),
		"INSERT INTO core.contacts (id, name, created_at) VALUES ($1, $2, now())",
		contactID, "Maria Perez"); err != nil {
		t.Fatalf("seeding the contact: %v", err)
	}
	if _, err := pool.Exec(t.Context(),
		"INSERT INTO core.contact_identities (id, contact_id, channel, identifier, display_name, created_at)"+
			" VALUES ($1, $2, 'phone', '184467235', '', now())",
		uuid.Must(uuid.NewV7()), contactID); err != nil {
		t.Fatalf("the first identity: %v", err)
	}

	_, err := pool.Exec(t.Context(),
		"INSERT INTO core.contact_identities (id, contact_id, channel, identifier, display_name, created_at)"+
			" VALUES ($1, $2, 'phone', '184467235', '', now())",
		uuid.Must(uuid.NewV7()), contactID)

	if err == nil {
		t.Error("a duplicate identity in one tenant was stored, want the composite to refuse it")
	}
}

func TestATenantNameIsHeldOnce(t *testing.T) {
	t.Parallel()

	pool := newTestPool(t)
	if _, err := pool.Exec(t.Context(),
		"INSERT INTO core.tenants (id, name) VALUES ($1, $2)",
		uuid.Must(uuid.NewV7()), "Acme"); err != nil {
		t.Fatalf("the first tenant: %v", err)
	}

	_, err := pool.Exec(t.Context(),
		"INSERT INTO core.tenants (id, name) VALUES ($1, $2)",
		uuid.Must(uuid.NewV7()), "Acme")

	if err == nil {
		t.Error("a second tenant under the same name was stored, want the unique to refuse it")
	}
}
