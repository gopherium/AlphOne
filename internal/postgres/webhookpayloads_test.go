// SPDX-License-Identifier: Elastic-2.0

package postgres_test

import (
	"context"
	"fmt"
	"slices"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/gopherium/alphone/internal/event"
	"github.com/gopherium/alphone/internal/postgres"
	"github.com/gopherium/alphone/internal/webhook"
)

// settledPayloadIndex names the index the clear of settled payloads walks.
const settledPayloadIndex = "webhook_deliveries_settled_payload_idx"

// storedDeliveryQuery reads what the queue holds for one delivery, an empty last error when it has none.
const storedDeliveryQuery = "SELECT payload, status, attempts, deliver_after, COALESCE(last_error, '') " +
	"FROM core.webhook_deliveries WHERE id = $1"

// subscriptionQueue stores the deliveries of one subscription in the tenant its ctx serves.
type subscriptionQueue struct {
	// ctx serves the tenant of the subscription.
	ctx context.Context
	// store holds the subscription and its deliveries.
	store *postgres.WebhookStore
	// sub is the subscription the deliveries go to.
	sub webhook.Subscription
}

// newSubscriptionQueue stores a subscription of no account in the tenant ctx serves and returns its queue.
func newSubscriptionQueue(t *testing.T, ctx context.Context, store *postgres.WebhookStore) subscriptionQueue {
	t.Helper()
	sub := mustSubscription(t, uuid.Must(uuid.NewV7()), "https://example.com/hook", event.TaskCreated)
	if err := store.CreateSubscription(ctx, sub); err != nil {
		t.Fatalf("CreateSubscription() error = %v, want nil", err)
	}
	return subscriptionQueue{ctx: ctx, store: store, sub: sub}
}

// enqueueAt stores a delivery of the subscription in status since at and returns it.
func (q subscriptionQueue) enqueueAt(t *testing.T, status string, at time.Time) webhook.Delivery {
	t.Helper()
	delivery := mustDelivery(t, q.sub)
	delivery.Status, delivery.DeliverAfter, delivery.CreatedAt, delivery.Attempts = status, at, at.Add(-time.Minute), 1
	return q.enqueue(t, delivery)
}

// enqueue stores delivery and returns it.
func (q subscriptionQueue) enqueue(t *testing.T, delivery webhook.Delivery) webhook.Delivery {
	t.Helper()
	if err := q.store.EnqueueDelivery(q.ctx, delivery); err != nil {
		t.Fatalf("EnqueueDelivery() error = %v, want nil", err)
	}
	return delivery
}

// storedDelivery is what the queue holds for one delivery, its time in UTC.
type storedDelivery struct {
	payload      string
	status       string
	attempts     int
	deliverAfter time.Time
	lastError    string
}

// String returns the stored delivery as a test failure shows it.
func (d storedDelivery) String() string {
	return fmt.Sprintf("%s since %v after %d attempts, last error %q, payload %q", d.status,
		d.deliverAfter.Format(time.RFC3339Nano), d.attempts, d.lastError, d.payload)
}

// storedDeliveryOf reads what the queue holds for the delivery called id.
func storedDeliveryOf(t *testing.T, pool *pgxpool.Pool, id uuid.UUID) storedDelivery {
	t.Helper()
	var held storedDelivery
	err := pool.QueryRow(t.Context(), storedDeliveryQuery, id).
		Scan(&held.payload, &held.status, &held.attempts, &held.deliverAfter, &held.lastError)
	if err != nil {
		t.Fatalf("reading the delivery %s: %v", id, err)
	}
	held.deliverAfter = held.deliverAfter.UTC()
	return held
}

// clearCase is one delivery a clear meets and whether it keeps its payload.
type clearCase struct {
	delivery webhook.Delivery
	kept     bool
}

// payloadAfterClear returns the payload the delivery holds after the clear.
func (c clearCase) payloadAfterClear() string {
	if c.kept {
		return string(c.delivery.Payload)
	}
	return ""
}

func TestSettlingADeliveryThatIsNoLongerPendingChangesNothing(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		held  string
		stale string
	}{
		"a sent delivery settled again as pending":            {held: webhook.StatusDelivered, stale: webhook.StatusPending},
		"a delivery failed for good settled again as pending": {held: webhook.StatusFailed, stale: webhook.StatusPending},
		"a sent delivery settled again as failed":             {held: webhook.StatusDelivered, stale: webhook.StatusFailed},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			pool := newTestPool(t)
			store := postgres.NewWebhookStore(pool)
			queue := newSubscriptionQueue(t, t.Context(), store)
			delivery := queue.enqueueAt(t, tc.held, time.Now().UTC().Add(-time.Hour))
			before := storedDeliveryOf(t, pool, delivery.ID)

			err := store.SettleDelivery(t.Context(), delivery.ID, tc.stale, time.Now().UTC().Add(time.Minute),
				"subscriber answered 500")

			if err != nil {
				t.Fatalf("SettleDelivery() error = %v, want nil", err)
			}
			if after := storedDeliveryOf(t, pool, delivery.ID); after != before {
				t.Errorf("the delivery holds %v after a late settle as %s, want %v kept", after, tc.stale, before)
			}
		})
	}
}

func TestWebhookStoreClearsOnlyThePayloadsSettledBeforeTheCutoff(t *testing.T) {
	t.Parallel()

	pool := newTestPool(t)
	store := postgres.NewWebhookStore(pool)
	here := newSubscriptionQueue(t, t.Context(), store)
	there := newSubscriptionQueue(t, standingIn(t, seededTenant(t, pool)), store)
	cutoff := time.Now().UTC().Add(-time.Hour)
	before, after := cutoff.Add(-time.Minute), cutoff.Add(time.Minute)
	tests := map[string]clearCase{
		"a delivered one":                     {delivery: here.enqueueAt(t, webhook.StatusDelivered, before)},
		"a failed one":                        {delivery: here.enqueueAt(t, webhook.StatusFailed, before)},
		"one of another tenant":               {delivery: there.enqueueAt(t, webhook.StatusDelivered, before)},
		"one settled after the cutoff":        {delivery: here.enqueueAt(t, webhook.StatusDelivered, after), kept: true},
		"a pending one due before the cutoff": {delivery: here.enqueueAt(t, webhook.StatusPending, before), kept: true},
	}

	count, err := store.ClearSettledPayloads(t.Context(), cutoff, 10)

	if err != nil || count != 3 {
		t.Fatalf("ClearSettledPayloads() = %d, %v, want the 3 settled before the cutoff, nil", count, err)
	}
	for name, tc := range tests {
		if got := storedDeliveryOf(t, pool, tc.delivery.ID).payload; got != tc.payloadAfterClear() {
			t.Errorf("%s holds %q, want %q", name, got, tc.payloadAfterClear())
		}
	}
}

func TestClearingAPayloadKeepsTheRestOfTheDelivery(t *testing.T) {
	t.Parallel()

	pool := newTestPool(t)
	store := postgres.NewWebhookStore(pool)
	queue := newSubscriptionQueue(t, t.Context(), store)
	settled := time.Now().UTC().Add(-2 * time.Hour).Truncate(time.Microsecond)
	delivery := mustDelivery(t, queue.sub)
	delivery.Status, delivery.DeliverAfter, delivery.Attempts = webhook.StatusFailed, settled, 7
	delivery.LastError, delivery.CreatedAt = "subscriber answered 500", settled.Add(-webhook.RetryWindow)
	queue.enqueue(t, delivery)

	if _, err := store.ClearSettledPayloads(t.Context(), time.Now().UTC(), 10); err != nil {
		t.Fatalf("ClearSettledPayloads() error = %v, want nil", err)
	}

	want := storedDelivery{status: webhook.StatusFailed, attempts: 7, deliverAfter: settled, lastError: delivery.LastError}
	if got := storedDeliveryOf(t, pool, delivery.ID); got != want {
		t.Errorf("the delivery holds %v, want %v", got, want)
	}
}

func TestWebhookStoreClearsTheOldestPayloadsUpToTheLimit(t *testing.T) {
	t.Parallel()

	pool := newTestPool(t)
	store := postgres.NewWebhookStore(pool)
	queue := newSubscriptionQueue(t, t.Context(), store)
	cutoff := time.Now().UTC().Add(-time.Hour)
	oldest := queue.enqueueAt(t, webhook.StatusDelivered, cutoff.Add(-3*time.Minute))
	older := queue.enqueueAt(t, webhook.StatusDelivered, cutoff.Add(-2*time.Minute))
	newest := queue.enqueueAt(t, webhook.StatusDelivered, cutoff.Add(-time.Minute))

	count, err := store.ClearSettledPayloads(t.Context(), cutoff, 2)
	if err != nil || count != 2 {
		t.Fatalf("ClearSettledPayloads() = %d, %v, want 2, nil", count, err)
	}

	first := []string{
		storedDeliveryOf(t, pool, oldest.ID).payload,
		storedDeliveryOf(t, pool, older.ID).payload,
		storedDeliveryOf(t, pool, newest.ID).payload,
	}
	if want := []string{"", "", string(newest.Payload)}; !slices.Equal(first, want) {
		t.Errorf("payloads after the first clear = %q, want the two oldest cleared and %q", first, want)
	}
	if count, err := store.ClearSettledPayloads(t.Context(), cutoff, 2); err != nil || count != 1 {
		t.Fatalf("second ClearSettledPayloads() = %d, %v, want 1, nil", count, err)
	}
	if got := storedDeliveryOf(t, pool, newest.ID).payload; got != "" {
		t.Errorf("the newest payload after the second clear = %s, want it cleared past the ones already empty", got)
	}
}

func TestClearingSettledPayloadsWalksTheirIndex(t *testing.T) {
	t.Parallel()

	capture := &queryCapture{}
	pool := newTracedPool(t, capture)
	store := postgres.NewWebhookStore(pool)
	if _, err := store.ClearSettledPayloads(t.Context(), time.Now().UTC(), 50); err != nil {
		t.Fatalf("ClearSettledPayloads() error = %v, want nil", err)
	}

	var walked []string
	for _, node := range planOf(t, pool, capture.last()).nodes() {
		if node.IndexName != "" {
			walked = append(walked, node.IndexName)
		}
	}

	if !slices.Contains(walked, settledPayloadIndex) {
		t.Errorf("indexes walked = %v, want %s", walked, settledPayloadIndex)
	}
}

func TestClearingSettledPayloadsReportsConnectionFailure(t *testing.T) {
	t.Parallel()

	pool := newTestPool(t)
	store := postgres.NewWebhookStore(pool)
	pool.Close()

	if _, err := store.ClearSettledPayloads(t.Context(), time.Now().UTC(), 10); err == nil {
		t.Error("ClearSettledPayloads() on closed pool error = nil, want error")
	}
}
