// SPDX-License-Identifier: Elastic-2.0

package whatsapp

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/gopherium/alphone/sdk"
)

// servingTenant returns a context serving one freshly seeded tenant.
func servingTenant(t *testing.T, p *Plugin) context.Context {
	t.Helper()
	return sdk.WithTenant(t.Context(), seededTenant(t, p, "Acme"))
}

// arrivedMessage stores one inbound message in the tenant the context serves.
func arrivedMessage(t *testing.T, p *Plugin, ctx context.Context, sender, wamid string) uuid.UUID {
	t.Helper()
	contactID := uuid.Must(uuid.NewV7())
	if _, err := p.pool.Exec(ctx,
		"INSERT INTO core.contacts (id, name, created_at, tenant_id) VALUES ($1, $2, now(), $3)",
		contactID, "Maria Perez", sdk.TenantOrDefault(ctx)); err != nil {
		t.Fatalf("seeding the contact: %v", err)
	}
	conversationID, _, err := p.store.persistInbound(ctx, contactID, inboundMessage{
		sender:      sender,
		externalID:  wamid,
		content:     "hello",
		contentType: "text",
		sentAt:      time.Now().UTC(),
		raw:         []byte(`{}`),
	})
	if err != nil {
		t.Fatalf("persisting the message: %v", err)
	}
	return conversationID
}

func TestAConversationListingStaysInsideItsTenant(t *testing.T) {
	t.Parallel()

	p := newMigratedPlugin(t)
	acme := servingTenant(t, p)
	arrivedMessage(t, p, acme, "184467235", "wamid.ACME")
	arrivedMessage(t, p, t.Context(), "184467236", "wamid.OTHER")

	listed, err := p.store.listConversations(acme, 10)

	if err != nil {
		t.Fatalf("listConversations() error = %v, want nil", err)
	}
	if len(listed) != 1 || listed[0].ExternalID != "184467235" {
		t.Errorf("listConversations() = %+v, want only the tenant's own conversation", listed)
	}
}

func TestAConversationStaysUnreadableFromAnotherTenant(t *testing.T) {
	t.Parallel()

	p := newMigratedPlugin(t)
	acme := servingTenant(t, p)
	held := arrivedMessage(t, p, acme, "184467235", "wamid.ACME")

	mine, err := p.store.getConversation(acme, held)
	if err != nil {
		t.Fatalf("getConversation() inside the tenant error = %v, want nil", err)
	}
	if mine.ID != held {
		t.Fatalf("getConversation() = %v, want the conversation %v", mine.ID, held)
	}

	theirs, err := p.store.getConversation(t.Context(), held)

	if err != nil {
		t.Fatalf("getConversation() from another tenant error = %v, want nil", err)
	}
	if theirs.ID != uuid.Nil {
		t.Errorf("getConversation() = %v from another tenant, want it withheld", theirs.ID)
	}
}

func TestMessagesStayInsideTheirTenant(t *testing.T) {
	t.Parallel()

	p := newMigratedPlugin(t)
	acme := servingTenant(t, p)
	held := arrivedMessage(t, p, acme, "184467235", "wamid.ACME")

	listed, err := p.store.listMessages(t.Context(), held, 10)

	if err != nil {
		t.Fatalf("listMessages() error = %v, want nil", err)
	}
	if len(listed) != 0 {
		t.Errorf("listMessages() = %+v from another tenant, want the messages withheld", listed)
	}
}

func TestAContactsConversationsStayInsideTheirTenant(t *testing.T) {
	t.Parallel()

	p := newMigratedPlugin(t)
	acme := seededTenant(t, p, "Acme")
	conversationID := seededConversation(t, p, "wamid.acme", acme)
	var contactID uuid.UUID
	if err := p.pool.QueryRow(t.Context(),
		"SELECT contact_id FROM plugin_whatsapp.conversations WHERE id = $1", conversationID,
	).Scan(&contactID); err != nil {
		t.Fatalf("loading the contact id: %v", err)
	}

	held, err := p.store.listConversationsByContactIDs(t.Context(), []uuid.UUID{contactID})

	if err != nil {
		t.Fatalf("listConversationsByContactIDs() error = %v, want nil", err)
	}
	if len(held) != 0 {
		t.Errorf("listConversationsByContactIDs() = %+v from another tenant, want them withheld", held)
	}
}

func TestAConversationWithholdsAContactOfAnotherTenant(t *testing.T) {
	t.Parallel()

	p := newMigratedPlugin(t)
	acme := seededTenant(t, p, "Acme")
	elsewhere := seededTenant(t, p, "Globex")
	contactID := uuid.Must(uuid.NewV7())
	if _, err := p.pool.Exec(t.Context(),
		"INSERT INTO core.contacts (id, name, created_at, tenant_id) VALUES ($1, $2, now(), $3)",
		contactID, "Maria Perez", acme); err != nil {
		t.Fatalf("seeding the contact: %v", err)
	}
	if _, err := p.pool.Exec(t.Context(),
		"INSERT INTO plugin_whatsapp.conversations"+
			" (id, contact_id, channel, external_id, status, last_activity_at, created_at, tenant_id)"+
			" VALUES ($1, $2, 'whatsapp', '184467235', 'open', now(), now(), $3)",
		uuid.Must(uuid.NewV7()), contactID, elsewhere); err != nil {
		t.Fatalf("seeding the crossed conversation: %v", err)
	}

	held, err := p.store.listConversations(sdk.WithTenant(t.Context(), elsewhere), 10)

	if err != nil {
		t.Fatalf("listConversations() error = %v, want nil", err)
	}
	for _, row := range held {
		if row.ContactName == "Maria Perez" {
			t.Error("listConversations() rendered a contact of another tenant, want it withheld")
		}
	}
}

func TestABroadcastStaysInsideItsTenant(t *testing.T) {
	t.Parallel()

	events := newBroadcaster()
	acme, globex := uuid.Must(uuid.NewV7()), uuid.Must(uuid.NewV7())
	near := events.subscribe(acme)
	far := events.subscribe(globex)

	events.broadcast(event{Conversation: uuid.Must(uuid.NewV7()), Tenant: acme})

	if len(near) != 1 {
		t.Errorf("the tenant's subscriber buffered %d events, want the broadcast", len(near))
	}
	if len(far) != 0 {
		t.Errorf("another tenant's subscriber buffered %d events, want none", len(far))
	}
}

func TestTwoTenantsKeepSeparateThreadsForOneNumber(t *testing.T) {
	t.Parallel()

	p := newMigratedPlugin(t)
	acme := servingTenant(t, p)
	mine := arrivedMessage(t, p, acme, "184467235", "wamid.ACME")
	theirs := arrivedMessage(t, p, t.Context(), "184467235", "wamid.OTHER")

	if mine == theirs {
		t.Error("both tenants share one conversation for the same number, want separate threads")
	}
}
