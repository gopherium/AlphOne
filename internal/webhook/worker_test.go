// SPDX-License-Identifier: Elastic-2.0

package webhook_test

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/gopherium/alphone/internal/event"
	"github.com/gopherium/alphone/internal/webhook"
)

type settlement struct {
	ID           uuid.UUID
	Status       string
	DeliverAfter time.Time
	LastError    string
}

type fakeWorkerQueue struct {
	mu       sync.Mutex
	pending  []webhook.ClaimedDelivery
	settled  []settlement
	claimErr error
	claims   int
}

func (q *fakeWorkerQueue) ClaimDueDeliveries(
	_ context.Context,
	_, _ time.Time,
	_ int,
) ([]webhook.ClaimedDelivery, error) {
	q.mu.Lock()
	defer q.mu.Unlock()
	q.claims++
	if q.claimErr != nil {
		return nil, q.claimErr
	}
	claimed := q.pending
	q.pending = nil
	return claimed, nil
}

func (q *fakeWorkerQueue) SettleDelivery(
	_ context.Context,
	id uuid.UUID,
	status string,
	deliverAfter time.Time,
	lastError string,
) error {
	q.mu.Lock()
	defer q.mu.Unlock()
	q.settled = append(q.settled, settlement{ID: id, Status: status, DeliverAfter: deliverAfter, LastError: lastError})
	return nil
}

func (q *fakeWorkerQueue) settlements() []settlement {
	q.mu.Lock()
	defer q.mu.Unlock()
	return append([]settlement(nil), q.settled...)
}

// claimed builds a delivery ready to hand to the worker.
func claimed(t *testing.T, url, secret string, attempts int) webhook.ClaimedDelivery {
	t.Helper()
	occurred, err := event.New(event.TaskCreated, map[string]any{"title": "Call Maria"})
	if err != nil {
		t.Fatalf("event.New() error = %v, want nil", err)
	}
	payload, err := occurred.Payload()
	if err != nil {
		t.Fatalf("Payload() error = %v, want nil", err)
	}
	return webhook.ClaimedDelivery{
		Delivery: webhook.Delivery{
			ID:        uuid.Must(uuid.NewV7()),
			EventID:   occurred.ID,
			EventName: occurred.Name,
			Payload:   payload,
			Attempts:  attempts,
			Status:    webhook.StatusPending,
		},
		URL:    url,
		Secret: secret,
	}
}

// newWorker returns a worker over queue, and the log it writes to.
func newWorker(queue *fakeWorkerQueue) (*webhook.Worker, *strings.Builder) {
	var logged strings.Builder
	logger := slog.New(slog.NewTextHandler(&logged, nil))
	return webhook.NewWorker(queue, logger), &logged
}

func TestWorkerSignsAndDeliversTheExactPayload(t *testing.T) {
	t.Parallel()

	var (
		mu        sync.Mutex
		gotBody   []byte
		gotHeader http.Header
	)
	subscriber := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		mu.Lock()
		gotBody, gotHeader = body, r.Header.Clone()
		mu.Unlock()
		w.WriteHeader(http.StatusOK)
	}))
	defer subscriber.Close()
	delivery := claimed(t, subscriber.URL, "whsec_example", 1)
	queue := &fakeWorkerQueue{pending: []webhook.ClaimedDelivery{delivery}}
	worker, _ := newWorker(queue)

	worker.Sweep(t.Context())

	mu.Lock()
	defer mu.Unlock()
	if string(gotBody) != string(delivery.Payload) {
		t.Errorf("body = %s, want the queued payload %s", gotBody, delivery.Payload)
	}
	want := webhook.Sign("whsec_example", delivery.Payload)
	if got := gotHeader.Get("X-AlphOne-Signature-256"); got != want {
		t.Errorf("signature = %q, want %q", got, want)
	}
	if got := gotHeader.Get("X-AlphOne-Event"); got != string(event.TaskCreated) {
		t.Errorf("event header = %q, want %q", got, event.TaskCreated)
	}
	if got := gotHeader.Get("X-AlphOne-Delivery"); got != delivery.ID.String() {
		t.Errorf("delivery header = %q, want %q", got, delivery.ID)
	}
	if got := gotHeader.Get("Content-Type"); got != "application/json" {
		t.Errorf("content type = %q, want application/json", got)
	}
}

func TestWorkerMarksASuccessfulDeliveryDone(t *testing.T) {
	t.Parallel()

	subscriber := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusAccepted)
	}))
	defer subscriber.Close()
	queue := &fakeWorkerQueue{pending: []webhook.ClaimedDelivery{claimed(t, subscriber.URL, "whsec_a", 1)}}
	worker, _ := newWorker(queue)

	worker.Sweep(t.Context())

	settled := queue.settlements()
	if len(settled) != 1 || settled[0].Status != webhook.StatusDelivered {
		t.Fatalf("settled %+v, want one delivered", settled)
	}
}

func TestWorkerRetriesARejectedDelivery(t *testing.T) {
	t.Parallel()

	subscriber := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer subscriber.Close()
	queue := &fakeWorkerQueue{pending: []webhook.ClaimedDelivery{claimed(t, subscriber.URL, "whsec_a", 1)}}
	worker, _ := newWorker(queue)
	before := time.Now().UTC()

	worker.Sweep(t.Context())

	settled := queue.settlements()
	if len(settled) != 1 {
		t.Fatalf("settled %d deliveries, want 1", len(settled))
	}
	if settled[0].Status != webhook.StatusPending {
		t.Errorf("status = %q, want it left pending for a retry", settled[0].Status)
	}
	if held := settled[0].DeliverAfter.Sub(before); held < webhook.Backoff(1) {
		t.Errorf("deliver_after is %v away, want at least the first backoff of %v", held, webhook.Backoff(1))
	}
	if settled[0].LastError == "" {
		t.Error("last_error is empty, want the rejection recorded")
	}
}

func TestWorkerGivesUpAfterTheAttemptBudget(t *testing.T) {
	t.Parallel()

	subscriber := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer subscriber.Close()
	spent := claimed(t, subscriber.URL, "whsec_a", webhook.MaxAttempts)
	queue := &fakeWorkerQueue{pending: []webhook.ClaimedDelivery{spent}}
	worker, _ := newWorker(queue)

	worker.Sweep(t.Context())

	settled := queue.settlements()
	if len(settled) != 1 || settled[0].Status != webhook.StatusFailed {
		t.Fatalf("settled %+v, want one failed", settled)
	}
}

func TestWorkerRetriesAnUnreachableSubscriber(t *testing.T) {
	t.Parallel()

	queue := &fakeWorkerQueue{pending: []webhook.ClaimedDelivery{
		claimed(t, "http://127.0.0.1:9/gone", "whsec_a", 1),
	}}
	worker, _ := newWorker(queue)

	worker.Sweep(t.Context())

	settled := queue.settlements()
	if len(settled) != 1 || settled[0].Status != webhook.StatusPending {
		t.Fatalf("settled %+v, want one left pending", settled)
	}
	if settled[0].LastError == "" {
		t.Error("last_error is empty, want the transport failure recorded")
	}
}

func TestWorkerSurvivesAClaimFailure(t *testing.T) {
	t.Parallel()

	queue := &fakeWorkerQueue{claimErr: errors.New("queue unavailable")}
	worker, logged := newWorker(queue)

	worker.Sweep(t.Context())

	if !strings.Contains(logged.String(), "level=ERROR") {
		t.Errorf("log = %q, want the failure recorded", logged.String())
	}
}

func TestWorkerSweepsOnStartAndStops(t *testing.T) {
	t.Parallel()

	subscriber := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer subscriber.Close()
	queue := &fakeWorkerQueue{pending: []webhook.ClaimedDelivery{claimed(t, subscriber.URL, "whsec_a", 1)}}
	worker, _ := newWorker(queue)

	worker.Start()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if len(queue.settlements()) > 0 {
			break
		}
		time.Sleep(5 * time.Millisecond)
	}
	worker.Stop()

	if got := queue.settlements(); len(got) != 1 || got[0].Status != webhook.StatusDelivered {
		t.Errorf("settled %+v, want the queue drained on start", got)
	}
}

func TestWorkerSweepsWhenPoked(t *testing.T) {
	t.Parallel()

	subscriber := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer subscriber.Close()
	queue := &fakeWorkerQueue{}
	worker, _ := newWorker(queue)
	worker.Start()
	defer worker.Stop()

	queue.mu.Lock()
	queue.pending = []webhook.ClaimedDelivery{claimed(t, subscriber.URL, "whsec_a", 1)}
	queue.mu.Unlock()
	worker.Poke()

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if len(queue.settlements()) > 0 {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Error("a poke did not drain the queue")
}
