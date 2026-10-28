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
	"github.com/gopherium/alphone/internal/task"
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

// storedOwnerIn stores a contact owning the address in the tenant the context serves.
func storedOwnerIn(t *testing.T, store *postgres.ContactStore, ctx context.Context, owned ownedAddress) uuid.UUID {
	t.Helper()
	owner := storedContact(t, store, ctx, owned.name)
	if err := store.AddIdentity(ctx, mustIdentity(t, owner, owned.channel, owned.identifier)); err != nil {
		t.Fatalf("AddIdentity(%s) error = %v, want nil", owned.identifier, err)
	}
	return owner
}

func TestAMatchStaysInsideItsTenant(t *testing.T) {
	t.Parallel()

	pool := newTestPool(t)
	store := postgres.NewContactStore(pool)
	matcher := contact.NewMatcher(store, matchBound)
	acme := standingIn(t, seededTenant(t, pool))
	mine := storedOwnerIn(t, store, acme, ownedAddress{"Maria Perez", "email", "maria@example.com"})
	theirs := storedOwnerIn(t, store, t.Context(), ownedAddress{"Ada Lovelace", "email", "ada@example.com"})
	shared := storedOwnerIn(t, store, t.Context(), ownedAddress{"Grace Hopper", "email", "maria@example.com"})

	inside, err := matcher.MatchIdentities(acme, "email", []string{"maria@example.com", "ada@example.com"})
	if err != nil {
		t.Fatalf("MatchIdentities() inside Acme error = %v, want nil", err)
	}
	elsewhere, err := matcher.MatchIdentities(t.Context(), "email", []string{"maria@example.com", "ada@example.com"})
	if err != nil {
		t.Fatalf("MatchIdentities() elsewhere error = %v, want nil", err)
	}

	if len(inside) != 1 || inside["maria@example.com"].ID != mine {
		t.Errorf("MatchIdentities() inside Acme = %+v, want only its own owner of maria@example.com", inside)
	}
	if elsewhere["maria@example.com"].ID != shared || elsewhere["ada@example.com"].ID != theirs {
		t.Errorf("MatchIdentities() elsewhere = %+v, want the other tenant's own owners", elsewhere)
	}
}

func TestTheIdentityFeedStaysInsideItsTenant(t *testing.T) {
	t.Parallel()

	pool := newTestPool(t)
	store := postgres.NewContactStore(pool)
	feed := contact.NewFeed(store, feedPageLimit)
	acme := standingIn(t, seededTenant(t, pool))
	theirs := storedOwnerIn(t, store, t.Context(), ownedAddress{"Ada Lovelace", "email", "ada@example.com"})
	mine := storedOwnerIn(t, store, acme, ownedAddress{"Maria Perez", "email", "maria@example.com"})

	inside, _ := feedReader{t, acme, feed}.settled(contact.Cursor{}, 1)
	elsewhere, _ := feedReader{t, t.Context(), feed}.settled(contact.Cursor{}, 1)

	if len(inside) != 1 || inside[0].ContactID != mine {
		t.Errorf("the Acme feed answered %+v, want only its own identity", inside)
	}
	if len(elsewhere) != 1 || elsewhere[0].ContactID != theirs {
		t.Errorf("the other tenant's feed answered %+v, want only its own identity", elsewhere)
	}
}

func TestAUserSettingStaysInsideItsTenant(t *testing.T) {
	t.Parallel()

	pool := newTestPool(t)
	elsewhere := readerOver(t, pool)
	acme := elsewhere.in(standingIn(t, seededTenant(t, pool)))
	acme.save("locale.default", "es-ES")

	elsewhere.save("locale.default", "en-GB")

	acme.want("locale.default", foundAs("es-ES"))
	elsewhere.want("locale.default", foundAs("en-GB"))
}

func TestAWorkspaceSettingStaysInsideItsTenant(t *testing.T) {
	t.Parallel()

	pool := newTestPool(t)
	store := postgres.NewSettingStore(pool, settingKeys(t, 1)).ForPlugin("email")
	acme := settingsIn{t: t, ctx: standingIn(t, seededTenant(t, pool)), store: store}
	elsewhere := settingsIn{t: t, ctx: t.Context(), store: store}
	acme.save(map[string]string{"imap.host": "imap.example.com"})

	elsewhere.save(map[string]string{"imap.host": "mail.example.com"})

	acme.wantPlain("imap.host", "imap.example.com")
	elsewhere.wantPlain("imap.host", "mail.example.com")
}

func TestATaskOriginReplaysInsideItsTenantAlone(t *testing.T) {
	t.Parallel()

	pool := newTestPool(t)
	store := postgres.NewTaskStore(pool)
	acme := seededTenant(t, pool)
	assignee := uuid.Must(uuid.NewV7())
	origin := task.Origin{Source: "n8n", EventID: uuid.Must(uuid.NewV7())}
	held := task.Task{
		ID: uuid.Must(uuid.NewV7()), AssigneeID: assignee, Title: "In Acme",
		Status: "open", Priority: 2, Origin: origin, CreatedAt: time.Now(),
	}
	if _, _, err := store.Create(standingIn(t, acme), held); err != nil {
		t.Fatalf("Create() in Acme error = %v, want nil", err)
	}

	elsewhere := held
	elsewhere.ID = uuid.Must(uuid.NewV7())
	elsewhere.Title = "Elsewhere"
	got, created, err := store.Create(t.Context(), elsewhere)

	if err != nil {
		t.Fatalf("Create() elsewhere error = %v, want the same origin admitted", err)
	}
	if !created {
		t.Error("the replay guard fired across tenants, want the other tenant to get its own task")
	}
	if got.Title == "In Acme" {
		t.Error("Create() elsewhere answered the Acme task, want the other tenant's own")
	}
}

func TestAWebhookSubscriptionFiresForItsTenantAlone(t *testing.T) {
	t.Parallel()

	pool := newTestPool(t)
	store := postgres.NewWebhookStore(pool)
	acme := seededTenant(t, pool)
	mine := standingIn(t, acme)
	sub, err := webhook.NewSubscription(
		storedOwner(t, pool, "maria@example.com"), "https://example.com/hook", []event.Name{event.TaskCreated},
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
