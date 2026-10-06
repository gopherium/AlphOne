// SPDX-License-Identifier: Elastic-2.0

package graphres_test

import (
	"fmt"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/gopherium/gouncer"
	"github.com/gopherium/gouncer/authkit"
	authkitpg "github.com/gopherium/gouncer/authkit/postgres"

	"github.com/gopherium/alphone/internal/event"
	"github.com/gopherium/alphone/internal/graphres"
	"github.com/gopherium/alphone/internal/postgres"
	"github.com/gopherium/alphone/internal/role"
	"github.com/gopherium/alphone/internal/webhook"
)

// webhookOwner stores an account at email to own webhooks and answers its id.
func webhookOwner(t *testing.T, pool *pgxpool.Pool, email string) uuid.UUID {
	t.Helper()
	held, err := gouncer.NewInvitedUser(email, "Maria Perez")
	if err != nil {
		t.Fatalf("gouncer.NewInvitedUser() error = %v, want nil", err)
	}
	if err := authkitpg.NewUserStore(pool).CreateUser(t.Context(), held); err != nil {
		t.Fatalf("CreateUser() error = %v, want nil", err)
	}
	return held.ID
}

// ownedWebhook stores a webhook owned by owner and answers its id.
func ownedWebhook(t *testing.T, store *postgres.WebhookStore, owner uuid.UUID, address string) uuid.UUID {
	t.Helper()
	sub, err := webhook.NewSubscription(owner, address, []event.Name{event.TaskCreated})
	if err != nil {
		t.Fatalf("webhook.NewSubscription() error = %v, want nil", err)
	}
	if err := store.CreateSubscription(t.Context(), sub); err != nil {
		t.Fatalf("CreateSubscription() error = %v, want nil", err)
	}
	return sub.ID
}

// holderListing is the webhook listing a test reads back with each owner.
type holderListing struct {
	Webhooks []struct {
		URL   string `json:"url"`
		Owner *struct {
			ID    string `json:"id"`
			Name  string `json:"name"`
			Email string `json:"email"`
		} `json:"owner"`
	} `json:"webhooks"`
}

func TestAWebhookHolderListsEveryWebhookOfTheWorkspaceWithItsOwner(t *testing.T) {
	t.Parallel()

	pool := newTestPool(t)
	store := postgres.NewWebhookStore(pool)
	member := webhookOwner(t, pool, "member@example.com")
	ownedWebhook(t, store, member, "https://example.com/member")
	resolver := &graphres.Resolver{Version: "9.9.9", Webhooks: store}
	admin := newActingClient(t, resolver, authkit.Identity{ID: uuid.Must(uuid.NewV7()), Role: role.Admin.String()})

	var listed holderListing
	admin.MustPost(`{ webhooks { url owner { id name email } } }`, &listed)

	if len(listed.Webhooks) != 1 || listed.Webhooks[0].URL != "https://example.com/member" {
		t.Fatalf("webhooks = %+v, want the member's webhook listed to the admin", listed.Webhooks)
	}
	owner := listed.Webhooks[0].Owner
	if owner == nil || owner.ID != member.String() || owner.Name != "Maria Perez" || owner.Email != "member@example.com" {
		t.Errorf("owner = %+v, want the member's id, name and email", owner)
	}
}

func TestCreatingAWebhookAnswersItsOwner(t *testing.T) {
	t.Parallel()

	resolver := &graphres.Resolver{Version: "9.9.9", Webhooks: postgres.NewWebhookStore(newTestPool(t))}
	caller := authkit.Identity{
		ID: uuid.Must(uuid.NewV7()), Name: "Maria Perez", Email: "admin@example.com", Role: role.Admin.String(),
	}
	client := newActingClient(t, resolver, caller)

	var created struct {
		CreateWebhook struct {
			Webhook struct {
				Owner *struct {
					ID    string `json:"id"`
					Name  string `json:"name"`
					Email string `json:"email"`
				} `json:"owner"`
			} `json:"webhook"`
		} `json:"createWebhook"`
	}
	client.MustPost(`mutation { createWebhook(url: "https://example.com/hook", events: ["task.created"])
		{ webhook { owner { id name email } } } }`, &created)

	owner := created.CreateWebhook.Webhook.Owner
	if owner == nil || owner.ID != caller.ID.String() || owner.Name != caller.Name || owner.Email != caller.Email {
		t.Errorf("owner = %+v, want the caller's id, name and email", owner)
	}
}

func TestAWebhookHolderDeletesAnotherAccountsWebhook(t *testing.T) {
	t.Parallel()

	pool := newTestPool(t)
	store := postgres.NewWebhookStore(pool)
	member := webhookOwner(t, pool, "member@example.com")
	id := ownedWebhook(t, store, member, "https://example.com/member")
	resolver := &graphres.Resolver{Version: "9.9.9", Webhooks: store}
	admin := newActingClient(t, resolver, authkit.Identity{ID: uuid.Must(uuid.NewV7()), Role: role.Admin.String()})

	var deleted struct {
		DeleteWebhook bool `json:"deleteWebhook"`
	}
	admin.MustPost(fmt.Sprintf(`mutation { deleteWebhook(id: %q) }`, id), &deleted)

	if !deleted.DeleteWebhook {
		t.Error("deleteWebhook = false, want the admin to delete the member's webhook")
	}
	left, err := store.ListWorkspaceSubscriptions(t.Context())
	if err != nil || len(left) != 0 {
		t.Errorf("ListWorkspaceSubscriptions() = %+v, %v, want none left", left, err)
	}
}

func TestAnAccountWithoutTheCapabilityManagesOnlyItsOwnWebhooks(t *testing.T) {
	t.Parallel()

	pool := newTestPool(t)
	store := postgres.NewWebhookStore(pool)
	member := webhookOwner(t, pool, "member@example.com")
	other := webhookOwner(t, pool, "other@example.com")
	ownedWebhook(t, store, member, "https://example.com/member")
	foreign := ownedWebhook(t, store, other, "https://example.com/other")
	resolver := &graphres.Resolver{Version: "9.9.9", Webhooks: store}
	client := newActingClient(t, resolver, authkit.Identity{ID: member, Role: role.Member.String()})

	var listed holderListing
	client.MustPost(`{ webhooks { url owner { id name email } } }`, &listed)
	refused, err := client.RawPost(fmt.Sprintf(`mutation { deleteWebhook(id: %q) }`, foreign))

	if len(listed.Webhooks) != 1 || listed.Webhooks[0].URL != "https://example.com/member" {
		t.Errorf("webhooks = %+v, want only the member's own", listed.Webhooks)
	}
	if err != nil {
		t.Fatalf("RawPost() error = %v, want nil", err)
	}
	if got := firstErrorCode(t, refused.Errors); got != "NOT_FOUND" {
		t.Errorf("deleting another account's webhook code = %q, want NOT_FOUND", got)
	}
}
