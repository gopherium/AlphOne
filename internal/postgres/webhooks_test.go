// SPDX-License-Identifier: Elastic-2.0

package postgres_test

import (
	"errors"
	"log/slog"
	"reflect"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	authkitpg "github.com/gopherium/gouncer/authkit/postgres"

	"github.com/gopherium/alphone/internal/event"
	"github.com/gopherium/alphone/internal/postgres"
	"github.com/gopherium/alphone/internal/webhook"
)

// mustSubscription returns a subscription for the owner, url and event names, failing the test on error.
func mustSubscription(t *testing.T, owner uuid.UUID, url string, events ...event.Name) webhook.Subscription {
	t.Helper()
	sub, err := webhook.NewSubscription(owner, url, events)
	if err != nil {
		t.Fatalf("webhook.NewSubscription() error = %v, want nil", err)
	}
	return sub
}

// setOwnerDisabled disables or enables the owner's account, failing the test on error.
func setOwnerDisabled(t *testing.T, pool *pgxpool.Pool, owner uuid.UUID, disabled bool) {
	t.Helper()
	if err := authkitpg.NewUserStore(pool).SetUserDisabled(t.Context(), owner, disabled); err != nil {
		t.Fatalf("SetUserDisabled(%v) error = %v, want nil", disabled, err)
	}
}

// claimedNow claims every delivery due now and returns them.
func claimedNow(t *testing.T, store *postgres.WebhookStore) []webhook.ClaimedDelivery {
	t.Helper()
	now := time.Now().UTC()
	claimed, err := store.ClaimDueDeliveries(t.Context(), now, now.Add(time.Minute), 10)
	if err != nil {
		t.Fatalf("ClaimDueDeliveries() error = %v, want nil", err)
	}
	return claimed
}

// mustDelivery returns a delivery of a fresh task created event for the subscription, failing the test on error.
func mustDelivery(t *testing.T, sub webhook.Subscription) webhook.Delivery {
	t.Helper()
	occurred, err := event.New(event.TaskCreated, map[string]any{"id": uuid.Must(uuid.NewV7()).String()})
	if err != nil {
		t.Fatalf("event.New() error = %v, want nil", err)
	}
	delivery, err := webhook.NewDelivery(sub, occurred)
	if err != nil {
		t.Fatalf("webhook.NewDelivery() error = %v, want nil", err)
	}
	return delivery
}

func TestWebhookStoreSubscriptionRoundTrip(t *testing.T) {
	t.Parallel()

	store := postgres.NewWebhookStore(newTestPool(t))
	owner := uuid.Must(uuid.NewV7())
	sub := mustSubscription(t, owner, "https://example.com/hook", event.TaskCreated, event.ContactCreated)

	if err := store.CreateSubscription(t.Context(), sub); err != nil {
		t.Fatalf("CreateSubscription() error = %v, want nil", err)
	}

	got, err := store.ListSubscriptionsForUser(t.Context(), owner)
	if err != nil {
		t.Fatalf("ListSubscriptionsForUser() error = %v, want nil", err)
	}
	if len(got) != 1 {
		t.Fatalf("got %d subscriptions, want 1", len(got))
	}
	if diff := cmp.Diff(sub, got[0].Subscription, cmpopts.EquateApproxTime(time.Microsecond)); diff != "" {
		t.Errorf("subscription mismatch (-want +got):\n%s", diff)
	}
}

func TestWebhookStoreListsOnlyTheOwnersSubscriptions(t *testing.T) {
	t.Parallel()

	store := postgres.NewWebhookStore(newTestPool(t))
	owner := uuid.Must(uuid.NewV7())
	stranger := uuid.Must(uuid.NewV7())
	for _, sub := range []webhook.Subscription{
		mustSubscription(t, owner, "https://example.com/mine", event.TaskCreated),
		mustSubscription(t, stranger, "https://example.com/theirs", event.TaskCreated),
	} {
		if err := store.CreateSubscription(t.Context(), sub); err != nil {
			t.Fatalf("CreateSubscription() error = %v, want nil", err)
		}
	}

	got, err := store.ListSubscriptionsForUser(t.Context(), owner)

	if err != nil {
		t.Fatalf("ListSubscriptionsForUser() error = %v, want nil", err)
	}
	if len(got) != 1 || got[0].URL != "https://example.com/mine" {
		t.Errorf("got %d subscriptions, want only the owner's", len(got))
	}
}

func TestWebhookStoreMatchesSubscriptionsByEvent(t *testing.T) {
	t.Parallel()

	pool := newTestPool(t)
	store := postgres.NewWebhookStore(pool)
	wanted := mustSubscription(t, storedOwner(t, pool, "maria@example.com"),
		"https://example.com/tasks", event.TaskCreated)
	other := mustSubscription(t, storedOwner(t, pool, "luis@example.com"),
		"https://example.com/contacts", event.ContactCreated)
	for _, sub := range []webhook.Subscription{wanted, other} {
		if err := store.CreateSubscription(t.Context(), sub); err != nil {
			t.Fatalf("CreateSubscription() error = %v, want nil", err)
		}
	}

	got, err := store.ListSubscriptionsForEvent(t.Context(), event.TaskCreated)

	if err != nil {
		t.Fatalf("ListSubscriptionsForEvent() error = %v, want nil", err)
	}
	if len(got) != 1 || got[0].ID != wanted.ID {
		t.Errorf("got %d subscriptions, want only the one subscribed to task.created", len(got))
	}
}

func TestWebhookStoreSkipsTheSubscriptionsOfADisabledOwner(t *testing.T) {
	t.Parallel()

	pool := newTestPool(t)
	store := postgres.NewWebhookStore(pool)
	owner := storedOwner(t, pool, "maria@example.com")
	sub := mustSubscription(t, owner, "https://example.com/hook", event.TaskCreated)
	if err := store.CreateSubscription(t.Context(), sub); err != nil {
		t.Fatalf("CreateSubscription() error = %v, want nil", err)
	}
	setOwnerDisabled(t, pool, owner, true)

	disabled, err := store.ListSubscriptionsForEvent(t.Context(), event.TaskCreated)

	if err != nil {
		t.Fatalf("ListSubscriptionsForEvent() error = %v, want nil", err)
	}
	if len(disabled) != 0 {
		t.Errorf("got %d subscriptions of a disabled owner, want none", len(disabled))
	}
	setOwnerDisabled(t, pool, owner, false)
	enabled, err := store.ListSubscriptionsForEvent(t.Context(), event.TaskCreated)
	if err != nil {
		t.Fatalf("ListSubscriptionsForEvent() after re-enabling error = %v, want nil", err)
	}
	if len(enabled) != 1 || enabled[0].ID != sub.ID {
		t.Errorf("got %d subscriptions once the owner is enabled again, want the one subscription back", len(enabled))
	}
}

func TestWebhookStoreSkipsASubscriptionWithoutAnOwnerAccount(t *testing.T) {
	t.Parallel()

	store := postgres.NewWebhookStore(newTestPool(t))
	orphan := mustSubscription(t, uuid.Must(uuid.NewV7()), "https://example.com/hook", event.TaskCreated)
	if err := store.CreateSubscription(t.Context(), orphan); err != nil {
		t.Fatalf("CreateSubscription() error = %v, want nil", err)
	}

	got, err := store.ListSubscriptionsForEvent(t.Context(), event.TaskCreated)

	if err != nil {
		t.Fatalf("ListSubscriptionsForEvent() error = %v, want nil", err)
	}
	if len(got) != 0 {
		t.Errorf("got %d subscriptions owned by no account, want none", len(got))
	}
}

func TestADisabledOwnersWebhookGetsNoDeliveryUntilEnabledAgain(t *testing.T) {
	t.Parallel()

	pool := newTestPool(t)
	store := postgres.NewWebhookStore(pool)
	dispatcher := webhook.NewDispatcher(store, slog.New(slog.DiscardHandler))
	owner := storedOwner(t, pool, "maria@example.com")
	sub := mustSubscription(t, owner, "https://example.com/hook", event.TaskCreated)
	if err := store.CreateSubscription(t.Context(), sub); err != nil {
		t.Fatalf("CreateSubscription() error = %v, want nil", err)
	}
	setOwnerDisabled(t, pool, owner, true)

	dispatcher.Publish(t.Context(), event.TaskCreated, map[string]any{"title": "Call Maria"})

	if got := claimedNow(t, store); len(got) != 0 {
		t.Errorf("queued %d deliveries for a disabled owner, want none", len(got))
	}
	setOwnerDisabled(t, pool, owner, false)
	dispatcher.Publish(t.Context(), event.TaskCreated, map[string]any{"title": "Call Maria again"})
	got := claimedNow(t, store)
	if len(got) != 1 || got[0].SubscriptionID != sub.ID {
		t.Errorf("queued %d deliveries once the owner is enabled again, want one for the subscription", len(got))
	}
}

func TestWebhookStoreReportsWhetherASubscriptionsOwnerIsDisabled(t *testing.T) {
	t.Parallel()

	for name, tc := range map[string]struct {
		disabled  bool
		ownerless bool
		missing   bool
		want      bool
	}{
		"an enabled owner":         {want: false},
		"a disabled owner":         {disabled: true, want: true},
		"an owner with no account": {ownerless: true, want: true},
		"a deleted subscription":   {missing: true, want: true},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			pool := newTestPool(t)
			store := postgres.NewWebhookStore(pool)
			owner := uuid.Must(uuid.NewV7())
			if !tc.ownerless {
				owner = storedOwner(t, pool, "maria@example.com")
			}
			sub := mustSubscription(t, owner, "https://example.com/hook", event.TaskCreated)
			if !tc.missing {
				if err := store.CreateSubscription(t.Context(), sub); err != nil {
					t.Fatalf("CreateSubscription() error = %v, want nil", err)
				}
			}
			if tc.disabled {
				setOwnerDisabled(t, pool, owner, true)
			}

			got, err := store.OwnerDisabled(t.Context(), sub.ID)

			if err != nil || got != tc.want {
				t.Errorf("OwnerDisabled() = %v, %v, want %v, nil", got, err, tc.want)
			}
		})
	}
}

func TestWebhookStoreReportsAFailedOwnerLookup(t *testing.T) {
	t.Parallel()

	pool := newTestPool(t)
	store := postgres.NewWebhookStore(pool)
	pool.Close()

	if _, err := store.OwnerDisabled(t.Context(), uuid.Must(uuid.NewV7())); err == nil {
		t.Error("OwnerDisabled() over a closed pool error = nil, want the failure")
	}
}

func TestWebhookStoreDeletesOnlyForItsOwner(t *testing.T) {
	t.Parallel()

	store := postgres.NewWebhookStore(newTestPool(t))
	owner := uuid.Must(uuid.NewV7())
	sub := mustSubscription(t, owner, "https://example.com/hook", event.TaskCreated)
	if err := store.CreateSubscription(t.Context(), sub); err != nil {
		t.Fatalf("CreateSubscription() error = %v, want nil", err)
	}

	err := store.DeleteSubscription(t.Context(), uuid.Must(uuid.NewV7()), sub.ID)
	if !errors.Is(err, webhook.ErrNotFound) {
		t.Errorf("DeleteSubscription() by a stranger error = %v, want %v", err, webhook.ErrNotFound)
	}
	if err := store.DeleteSubscription(t.Context(), owner, sub.ID); err != nil {
		t.Fatalf("DeleteSubscription() by the owner error = %v, want nil", err)
	}

	got, err := store.ListSubscriptionsForUser(t.Context(), owner)
	if err != nil {
		t.Fatalf("ListSubscriptionsForUser() error = %v, want nil", err)
	}
	if len(got) != 0 {
		t.Errorf("got %d subscriptions after delete, want 0", len(got))
	}
}

func TestWebhookStoreClaimsDueDeliveriesOnce(t *testing.T) {
	t.Parallel()

	store := postgres.NewWebhookStore(newTestPool(t))
	sub := mustSubscription(t, uuid.Must(uuid.NewV7()), "https://example.com/hook", event.TaskCreated)
	if err := store.CreateSubscription(t.Context(), sub); err != nil {
		t.Fatalf("CreateSubscription() error = %v, want nil", err)
	}
	delivery := mustDelivery(t, sub)
	if err := store.EnqueueDelivery(t.Context(), delivery); err != nil {
		t.Fatalf("EnqueueDelivery() error = %v, want nil", err)
	}
	now := time.Now().UTC()

	claimed, err := store.ClaimDueDeliveries(t.Context(), now, now.Add(time.Minute), 10)

	if err != nil {
		t.Fatalf("ClaimDueDeliveries() error = %v, want nil", err)
	}
	if len(claimed) != 1 {
		t.Fatalf("claimed %d deliveries, want 1", len(claimed))
	}
	if claimed[0].Attempts != 1 {
		t.Errorf("Attempts = %d, want the claim to count an attempt", claimed[0].Attempts)
	}
	if string(claimed[0].Payload) != string(delivery.Payload) {
		t.Errorf("Payload = %s, want %s", claimed[0].Payload, delivery.Payload)
	}

	again, err := store.ClaimDueDeliveries(t.Context(), now, now.Add(time.Minute), 10)
	if err != nil {
		t.Fatalf("second ClaimDueDeliveries() error = %v, want nil", err)
	}
	if len(again) != 0 {
		t.Errorf("claimed %d deliveries on the second sweep, want 0 while the lease holds", len(again))
	}
}

func TestWebhookStoreLeavesFutureDeliveriesAlone(t *testing.T) {
	t.Parallel()

	store := postgres.NewWebhookStore(newTestPool(t))
	sub := mustSubscription(t, uuid.Must(uuid.NewV7()), "https://example.com/hook", event.TaskCreated)
	if err := store.CreateSubscription(t.Context(), sub); err != nil {
		t.Fatalf("CreateSubscription() error = %v, want nil", err)
	}
	delivery := mustDelivery(t, sub)
	delivery.DeliverAfter = time.Now().UTC().Add(time.Hour)
	if err := store.EnqueueDelivery(t.Context(), delivery); err != nil {
		t.Fatalf("EnqueueDelivery() error = %v, want nil", err)
	}
	now := time.Now().UTC()

	claimed, err := store.ClaimDueDeliveries(t.Context(), now, now.Add(time.Minute), 10)

	if err != nil {
		t.Fatalf("ClaimDueDeliveries() error = %v, want nil", err)
	}
	if len(claimed) != 0 {
		t.Errorf("claimed %d deliveries, want none due yet", len(claimed))
	}
}

func TestWebhookStoreSettlesADelivery(t *testing.T) {
	t.Parallel()

	store := postgres.NewWebhookStore(newTestPool(t))
	sub := mustSubscription(t, uuid.Must(uuid.NewV7()), "https://example.com/hook", event.TaskCreated)
	if err := store.CreateSubscription(t.Context(), sub); err != nil {
		t.Fatalf("CreateSubscription() error = %v, want nil", err)
	}
	delivery := mustDelivery(t, sub)
	if err := store.EnqueueDelivery(t.Context(), delivery); err != nil {
		t.Fatalf("EnqueueDelivery() error = %v, want nil", err)
	}
	now := time.Now().UTC()

	err := store.SettleDelivery(t.Context(), delivery.ID, webhook.StatusDelivered, now, "")

	if err != nil {
		t.Fatalf("SettleDelivery() error = %v, want nil", err)
	}
	claimed, err := store.ClaimDueDeliveries(t.Context(), now.Add(time.Hour), now.Add(2*time.Hour), 10)
	if err != nil {
		t.Fatalf("ClaimDueDeliveries() error = %v, want nil", err)
	}
	if len(claimed) != 0 {
		t.Errorf("claimed %d deliveries, want a settled one left alone", len(claimed))
	}
}

func TestWebhookStoreDropsDeliveriesWithTheirSubscription(t *testing.T) {
	t.Parallel()

	store := postgres.NewWebhookStore(newTestPool(t))
	owner := uuid.Must(uuid.NewV7())
	sub := mustSubscription(t, owner, "https://example.com/hook", event.TaskCreated)
	if err := store.CreateSubscription(t.Context(), sub); err != nil {
		t.Fatalf("CreateSubscription() error = %v, want nil", err)
	}
	if err := store.EnqueueDelivery(t.Context(), mustDelivery(t, sub)); err != nil {
		t.Fatalf("EnqueueDelivery() error = %v, want nil", err)
	}

	if err := store.DeleteSubscription(t.Context(), owner, sub.ID); err != nil {
		t.Fatalf("DeleteSubscription() error = %v, want nil", err)
	}

	now := time.Now().UTC()
	claimed, err := store.ClaimDueDeliveries(t.Context(), now, now.Add(time.Minute), 10)
	if err != nil {
		t.Fatalf("ClaimDueDeliveries() error = %v, want nil", err)
	}
	if len(claimed) != 0 {
		t.Errorf("claimed %d deliveries, want them removed with the subscription", len(claimed))
	}
}

func TestWebhookStoreReportsConnectionFailure(t *testing.T) {
	t.Parallel()

	pool := newTestPool(t)
	store := postgres.NewWebhookStore(pool)
	owner := uuid.Must(uuid.NewV7())
	sub := mustSubscription(t, owner, "https://example.com/hook", event.TaskCreated)
	delivery := mustDelivery(t, sub)
	now := time.Now().UTC()
	pool.Close()

	if err := store.CreateSubscription(t.Context(), sub); err == nil {
		t.Error("CreateSubscription() on closed pool error = nil, want error")
	}
	if _, err := store.ListSubscriptionsForUser(t.Context(), owner); err == nil {
		t.Error("ListSubscriptionsForUser() on closed pool error = nil, want error")
	}
	if _, err := store.ListSubscriptionsForEvent(t.Context(), event.TaskCreated); err == nil {
		t.Error("ListSubscriptionsForEvent() on closed pool error = nil, want error")
	}
	if err := store.DeleteSubscription(t.Context(), owner, sub.ID); err == nil || errors.Is(err, webhook.ErrNotFound) {
		t.Errorf("DeleteSubscription() on closed pool error = %v, want a non-ErrNotFound error", err)
	}
	if err := store.EnqueueDelivery(t.Context(), delivery); err == nil {
		t.Error("EnqueueDelivery() on closed pool error = nil, want error")
	}
	if _, err := store.ClaimDueDeliveries(t.Context(), now, now, 10); err == nil {
		t.Error("ClaimDueDeliveries() on closed pool error = nil, want error")
	}
	if err := store.SettleDelivery(t.Context(), delivery.ID, webhook.StatusFailed, now, "boom"); err == nil {
		t.Error("SettleDelivery() on closed pool error = nil, want error")
	}
	if _, err := store.ListWorkspaceSubscriptions(t.Context()); err == nil {
		t.Error("ListWorkspaceSubscriptions() on closed pool error = nil, want error")
	}
	if err := store.DeleteWorkspaceSubscription(t.Context(), sub.ID); err == nil || errors.Is(err, webhook.ErrNotFound) {
		t.Errorf("DeleteWorkspaceSubscription() on closed pool error = %v, want a non-ErrNotFound error", err)
	}
}

func TestWebhookStoreListsEveryWorkspaceSubscriptionWithItsOwner(t *testing.T) {
	t.Parallel()

	pool := newTestPool(t)
	store := postgres.NewWebhookStore(pool)
	member := storedOwner(t, pool, "member@example.com")
	gone := uuid.Must(uuid.NewV7())
	for _, sub := range []webhook.Subscription{
		mustSubscription(t, member, "https://example.com/member", event.TaskCreated),
		mustSubscription(t, gone, "https://example.com/orphan", event.TaskCreated),
	} {
		if err := store.CreateSubscription(t.Context(), sub); err != nil {
			t.Fatalf("CreateSubscription() error = %v, want nil", err)
		}
	}

	got, err := store.ListWorkspaceSubscriptions(t.Context())

	if err != nil {
		t.Fatalf("ListWorkspaceSubscriptions() error = %v, want nil", err)
	}
	owners := map[string]*webhook.Owner{}
	for _, listed := range got {
		owners[listed.URL] = listed.Owner
	}
	want := &webhook.Owner{ID: member, Name: "Maria Perez", Email: "member@example.com"}
	if len(got) != 2 || !reflect.DeepEqual(owners["https://example.com/member"], want) {
		t.Errorf("listed %+v, want both subscriptions and the member's owned by %+v", got, want)
	}
	if owner, held := owners["https://example.com/orphan"]; !held || owner != nil {
		t.Errorf("orphan owner = %+v, want a subscription whose account is gone listed with no owner", owner)
	}
}

func TestWebhookStoreListsTheOwnersSubscriptionsWithTheOwner(t *testing.T) {
	t.Parallel()

	pool := newTestPool(t)
	store := postgres.NewWebhookStore(pool)
	member := storedOwner(t, pool, "member@example.com")
	if err := store.CreateSubscription(t.Context(),
		mustSubscription(t, member, "https://example.com/member", event.TaskCreated)); err != nil {
		t.Fatalf("CreateSubscription() error = %v, want nil", err)
	}

	got, err := store.ListSubscriptionsForUser(t.Context(), member)

	want := &webhook.Owner{ID: member, Name: "Maria Perez", Email: "member@example.com"}
	if err != nil || len(got) != 1 || !reflect.DeepEqual(got[0].Owner, want) {
		t.Errorf("ListSubscriptionsForUser() = %+v, %v, want one subscription owned by %+v", got, err, want)
	}
}

func TestWebhookStoreDeletesAnySubscriptionOfTheWorkspace(t *testing.T) {
	t.Parallel()

	store := postgres.NewWebhookStore(newTestPool(t))
	sub := mustSubscription(t, uuid.Must(uuid.NewV7()), "https://example.com/hook", event.TaskCreated)
	if err := store.CreateSubscription(t.Context(), sub); err != nil {
		t.Fatalf("CreateSubscription() error = %v, want nil", err)
	}

	if err := store.DeleteWorkspaceSubscription(t.Context(), sub.ID); err != nil {
		t.Fatalf("DeleteWorkspaceSubscription() error = %v, want nil", err)
	}
	err := store.DeleteWorkspaceSubscription(t.Context(), sub.ID)

	if !errors.Is(err, webhook.ErrNotFound) {
		t.Errorf("DeleteWorkspaceSubscription() of a deleted subscription error = %v, want %v", err, webhook.ErrNotFound)
	}
}
