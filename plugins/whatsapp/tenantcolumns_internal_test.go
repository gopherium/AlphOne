// SPDX-License-Identifier: AGPL-3.0-or-later

package whatsapp

import (
	"testing"

	"github.com/google/uuid"

	"github.com/gopherium/alphone/internal/tenant"
)

// whatsappScopedTables lists every plugin table that carries the tenant boundary.
var whatsappScopedTables = []string{"conversations", "messages", "media", "credentials"}

// seededConversation stores one contact and one conversation under the given tenant.
func seededConversation(t *testing.T, p *Plugin, externalID string, standing uuid.UUID) uuid.UUID {
	t.Helper()
	contactID := uuid.Must(uuid.NewV7())
	if _, err := p.pool.Exec(t.Context(),
		"INSERT INTO core.contacts (id, name, created_at, tenant_id) VALUES ($1, $2, now(), $3)",
		contactID, "Maria Perez", standing); err != nil {
		t.Fatalf("seeding the contact: %v", err)
	}
	conversationID := uuid.Must(uuid.NewV7())
	if _, err := p.pool.Exec(t.Context(),
		"INSERT INTO plugin_whatsapp.conversations"+
			" (id, contact_id, channel, external_id, status, last_activity_at, created_at, tenant_id)"+
			" VALUES ($1, $2, 'whatsapp', $3, 'open', now(), now(), $4)",
		conversationID, contactID, externalID, standing); err != nil {
		t.Fatalf("seeding the conversation: %v", err)
	}
	return conversationID
}

// seededTenant stores one tenant under the given name.
func seededTenant(t *testing.T, p *Plugin, name string) uuid.UUID {
	t.Helper()
	id := uuid.Must(uuid.NewV7())
	if _, err := p.pool.Exec(t.Context(),
		"INSERT INTO core.tenants (id, name) VALUES ($1, $2)", id, name); err != nil {
		t.Fatalf("seeding the tenant: %v", err)
	}
	return id
}

func TestEveryMessagingTableCarriesItsTenant(t *testing.T) {
	t.Parallel()

	p := newMigratedPlugin(t)

	for _, table := range whatsappScopedTables {
		var nullable string
		err := p.pool.QueryRow(t.Context(),
			"SELECT is_nullable FROM information_schema.columns"+
				" WHERE table_schema = 'plugin_whatsapp' AND table_name = $1"+
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

func TestTwoTenantsMayHoldTheSameConversation(t *testing.T) {
	t.Parallel()

	p := newMigratedPlugin(t)
	acme := seededTenant(t, p, "Acme")
	seededConversation(t, p, "184467235", tenant.DefaultID)

	seededConversation(t, p, "184467235", acme)
}

func TestOneTenantRefusesADuplicateConversation(t *testing.T) {
	t.Parallel()

	p := newMigratedPlugin(t)
	first := seededConversation(t, p, "184467235", tenant.DefaultID)
	contactID := uuid.Must(uuid.NewV7())
	if _, err := p.pool.Exec(t.Context(),
		"INSERT INTO core.contacts (id, name, created_at) VALUES ($1, $2, now())",
		contactID, "Ada Lovelace"); err != nil {
		t.Fatalf("seeding the contact: %v", err)
	}

	_, err := p.pool.Exec(t.Context(),
		"INSERT INTO plugin_whatsapp.conversations"+
			" (id, contact_id, channel, external_id, status, last_activity_at, created_at)"+
			" VALUES ($1, $2, 'whatsapp', '184467235', 'open', now(), now())",
		uuid.Must(uuid.NewV7()), contactID)

	if err == nil {
		t.Error("a duplicate conversation in one tenant was stored, want the composite to refuse it")
	}
	_ = first
}

func TestTwoTenantsMayHoldTheSameMessageID(t *testing.T) {
	t.Parallel()

	p := newMigratedPlugin(t)
	acme := seededTenant(t, p, "Acme")
	mine := seededConversation(t, p, "184467235", tenant.DefaultID)
	theirs := seededConversation(t, p, "184467236", acme)
	insert := "INSERT INTO plugin_whatsapp.messages" +
		" (id, conversation_id, external_id, direction, content, content_type," +
		" sent_at, raw, created_at, tenant_id)" +
		" VALUES ($1, $2, 'wamid.SAME', 'inbound', 'hello', 'text', now(), '{}', now(), $3)"
	if _, err := p.pool.Exec(t.Context(), insert,
		uuid.Must(uuid.NewV7()), mine, tenant.DefaultID); err != nil {
		t.Fatalf("the default tenant message: %v", err)
	}

	_, err := p.pool.Exec(t.Context(), insert, uuid.Must(uuid.NewV7()), theirs, acme)

	if err != nil {
		t.Errorf("the same message id under another tenant: %v, want it admitted", err)
	}
}

func TestOneTenantRefusesADuplicateMessageID(t *testing.T) {
	t.Parallel()

	p := newMigratedPlugin(t)
	mine := seededConversation(t, p, "184467235", tenant.DefaultID)
	insert := "INSERT INTO plugin_whatsapp.messages" +
		" (id, conversation_id, external_id, direction, content, content_type," +
		" sent_at, raw, created_at)" +
		" VALUES ($1, $2, 'wamid.SAME', 'inbound', 'hello', 'text', now(), '{}', now())"
	if _, err := p.pool.Exec(t.Context(), insert, uuid.Must(uuid.NewV7()), mine); err != nil {
		t.Fatalf("the first message: %v", err)
	}

	_, err := p.pool.Exec(t.Context(), insert, uuid.Must(uuid.NewV7()), mine)

	if err == nil {
		t.Error("a duplicate message id in one tenant was stored, want the composite to refuse it")
	}
}
