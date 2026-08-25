// SPDX-License-Identifier: Elastic-2.0

package postgres_test

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/gopherium/alphone/internal/contact"
	"github.com/gopherium/alphone/internal/event"
	"github.com/gopherium/alphone/internal/postgres"
	"github.com/gopherium/alphone/internal/webhook"
	"github.com/gopherium/alphone/sdk"
)

// standingIn returns a context serving the named tenant.
func standingIn(t *testing.T, standing uuid.UUID) context.Context {
	t.Helper()
	return sdk.WithTenant(t.Context(), standing)
}

// seededTenant stores one tenant named Acme.
func seededTenant(t *testing.T, pool *pgxpool.Pool) uuid.UUID {
	t.Helper()
	id := uuid.Must(uuid.NewV7())
	if _, err := pool.Exec(t.Context(),
		"INSERT INTO core.tenants (id, name) VALUES ($1, 'Acme')", id); err != nil {
		t.Fatalf("seeding the tenant: %v", err)
	}
	return id
}

// storedContact stores one contact in the tenant the context serves.
func storedContact(t *testing.T, store *postgres.ContactStore, ctx context.Context, name string) uuid.UUID {
	t.Helper()
	held := contact.Contact{ID: uuid.Must(uuid.NewV7()), Name: name, CreatedAt: time.Now()}
	if err := store.Create(ctx, held); err != nil {
		t.Fatalf("storing the contact: %v", err)
	}
	return held.ID
}

func TestAContactStaysInsideItsTenant(t *testing.T) {
	t.Parallel()

	pool := newTestPool(t)
	store := postgres.NewContactStore(pool)
	acme := seededTenant(t, pool)
	mine := standingIn(t, acme)
	held := storedContact(t, store, mine, "Maria Perez")

	if _, err := store.Get(mine, held); err != nil {
		t.Fatalf("Get() inside the tenant error = %v, want the contact", err)
	}
	if _, err := store.Get(t.Context(), held); err == nil {
		t.Error("Get() from another tenant answered the contact, want it withheld")
	}
}

func TestAContactListingStaysInsideItsTenant(t *testing.T) {
	t.Parallel()

	pool := newTestPool(t)
	store := postgres.NewContactStore(pool)
	acme := seededTenant(t, pool)
	storedContact(t, store, standingIn(t, acme), "Maria Perez")
	storedContact(t, store, t.Context(), "Ada Lovelace")

	listed, err := store.ListContacts(standingIn(t, acme), "", "", "", uuid.Nil, 10)

	if err != nil {
		t.Fatalf("ListContacts() error = %v, want nil", err)
	}
	if len(listed) != 1 || listed[0].Name != "Maria Perez" {
		t.Errorf("ListContacts() = %+v, want only the tenant's own contact", listed)
	}
}

func TestAnIdentityResolvesInsideItsTenantAlone(t *testing.T) {
	t.Parallel()

	pool := newTestPool(t)
	store := postgres.NewContactStore(pool)
	acme := seededTenant(t, pool)
	mine := storedContact(t, store, standingIn(t, acme), "Maria Perez")
	theirs := storedContact(t, store, t.Context(), "Ada Lovelace")
	held := contact.Identity{
		ID:         uuid.Must(uuid.NewV7()),
		ContactID:  mine,
		Channel:    "phone",
		Identifier: "184467235",
		CreatedAt:  time.Now(),
	}
	if err := store.AddIdentity(standingIn(t, acme), held); err != nil {
		t.Fatalf("AddIdentity() in Acme error = %v, want nil", err)
	}

	if _, err := store.LookupIdentity(t.Context(), "phone", "184467235"); err == nil {
		t.Error("LookupIdentity() from another tenant answered, want the identity withheld")
	}
	theirs2 := held
	theirs2.ID = uuid.Must(uuid.NewV7())
	theirs2.ContactID = theirs
	if err := store.AddIdentity(t.Context(), theirs2); err != nil {
		t.Errorf("AddIdentity() of the same identifier elsewhere error = %v, want it admitted", err)
	}
}

func TestAWebhookSubscriptionFiresForItsTenantAlone(t *testing.T) {
	t.Parallel()

	pool := newTestPool(t)
	store := postgres.NewWebhookStore(pool)
	acme := seededTenant(t, pool)
	mine := standingIn(t, acme)
	sub, err := webhook.NewSubscription(
		uuid.Must(uuid.NewV7()), "https://example.com/hook", []event.Name{event.TaskCreated},
	)
	if err != nil {
		t.Fatalf("webhook.NewSubscription() error = %v, want nil", err)
	}
	if err := store.CreateSubscription(mine, sub); err != nil {
		t.Fatalf("CreateSubscription() in Acme error = %v, want nil", err)
	}

	inside, err := store.ListSubscriptionsForEvent(mine, event.TaskCreated)
	if err != nil {
		t.Fatalf("ListSubscriptionsForEvent() inside the tenant error = %v, want nil", err)
	}
	if len(inside) != 1 {
		t.Errorf("got %d subscriptions inside the tenant, want 1", len(inside))
	}
	outside, err := store.ListSubscriptionsForEvent(t.Context(), event.TaskCreated)
	if err != nil {
		t.Fatalf("ListSubscriptionsForEvent() from another tenant error = %v, want nil", err)
	}
	if len(outside) != 0 {
		t.Error("ListSubscriptionsForEvent() from another tenant answered the subscription, want it withheld")
	}
}
