// SPDX-License-Identifier: Elastic-2.0

package postgres_test

import (
	"errors"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	"github.com/google/uuid"

	"github.com/gopherium/alphone/internal/event"
	"github.com/gopherium/alphone/internal/postgres"
	"github.com/gopherium/alphone/internal/webhook"
)

func mustSubscription(t *testing.T, owner uuid.UUID, url string, events ...event.Name) webhook.Subscription {
	t.Helper()
	sub, err := webhook.NewSubscription(owner, url, events)
	if err != nil {
		t.Fatalf("webhook.NewSubscription() error = %v, want nil", err)
	}
	return sub
}

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
	if diff := cmp.Diff(sub, got[0], cmpopts.EquateApproxTime(time.Microsecond)); diff != "" {
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

	store := postgres.NewWebhookStore(newTestPool(t))
	wanted := mustSubscription(t, uuid.Must(uuid.NewV7()), "https://example.com/tasks", event.TaskCreated)
	other := mustSubscription(t, uuid.Must(uuid.NewV7()), "https://example.com/contacts", event.ContactCreated)
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
}
