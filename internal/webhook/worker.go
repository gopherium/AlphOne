// SPDX-License-Identifier: Elastic-2.0

package webhook

import (
	"bytes"
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/google/uuid"
)

// Delivery worker defaults.
const (
	// sweepInterval is how often the queue is checked for due deliveries.
	sweepInterval = 30 * time.Second
	// sweepBatch caps how many deliveries one sweep takes.
	sweepBatch = 50
	// sweepLease is how long a claimed delivery stays out before it is due
	// again.
	sweepLease = 5 * time.Minute
	// requestTimeout bounds one attempt against a subscriber.
	requestTimeout = 10 * time.Second
)

// WorkerQueue hands out due deliveries and records their outcome.
type WorkerQueue interface {
	ClaimDueDeliveries(ctx context.Context, due, lease time.Time, limit int) ([]ClaimedDelivery, error)
	SettleDelivery(ctx context.Context, id uuid.UUID, status string, deliverAfter time.Time, lastError string) error
}

// ClaimedDelivery is a delivery together with where to send it.
type ClaimedDelivery struct {
	Delivery
	URL    string
	Secret string
}

// Worker posts queued deliveries to their subscribers until each is
// accepted or has spent its attempt budget.
type Worker struct {
	queue    WorkerQueue
	client   *http.Client
	logger   *slog.Logger
	nudge    chan struct{}
	interval time.Duration
	cancel   context.CancelFunc
	done     chan struct{}
}

// NewWorker returns a [Worker] draining queue.
func NewWorker(queue WorkerQueue, logger *slog.Logger) *Worker {
	return &Worker{
		queue:    queue,
		client:   &http.Client{Timeout: requestTimeout},
		logger:   logger,
		nudge:    make(chan struct{}, 1),
		interval: sweepInterval,
	}
}

// Start begins draining the queue in the background.
func (w *Worker) Start() {
	ctx, cancel := context.WithCancel(context.Background())
	w.cancel = cancel
	w.done = make(chan struct{})
	go func() {
		defer close(w.done)
		w.run(ctx)
	}()
}

// Stop ends the delivery loop and waits for it to finish. Stopping a never
// started worker is not an error.
func (w *Worker) Stop() {
	if w.cancel == nil {
		return
	}
	w.cancel()
	<-w.done
}

// Poke wakes the delivery loop for a freshly queued delivery without waiting
// for the next tick.
func (w *Worker) Poke() {
	select {
	case w.nudge <- struct{}{}:
	default:
	}
}

// run sweeps once immediately, then on every tick or nudge until ctx ends.
func (w *Worker) run(ctx context.Context) {
	w.Sweep(ctx)
	ticker := time.NewTicker(w.interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			w.Sweep(ctx)
		case <-w.nudge:
			w.Sweep(ctx)
		}
	}
}

// Sweep attempts every delivery currently due.
func (w *Worker) Sweep(ctx context.Context) {
	now := time.Now().UTC()
	claimed, err := w.queue.ClaimDueDeliveries(ctx, now, now.Add(sweepLease), sweepBatch)
	if err != nil {
		w.logger.ErrorContext(ctx, "claiming webhook deliveries", "error", err)
		return
	}
	for _, delivery := range claimed {
		w.attempt(ctx, delivery)
	}
}

// attempt posts one delivery and records what happened.
func (w *Worker) attempt(ctx context.Context, d ClaimedDelivery) {
	err := w.post(ctx, d)
	if err == nil {
		w.settle(ctx, d, StatusDelivered, time.Now().UTC(), "")
		return
	}
	if Exhausted(d.Attempts) {
		w.logger.ErrorContext(ctx, "giving up on a webhook delivery",
			"delivery", d.ID, "attempts", d.Attempts, "error", err)
		w.settle(ctx, d, StatusFailed, time.Now().UTC(), err.Error())
		return
	}
	w.settle(ctx, d, StatusPending, time.Now().UTC().Add(Backoff(d.Attempts)), err.Error())
}

// post hands the signed payload to the subscriber, reporting any response
// outside the 2xx range as a failure.
func (w *Worker) post(ctx context.Context, d ClaimedDelivery) error {
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, d.URL, bytes.NewReader(d.Payload))
	if err != nil {
		return fmt.Errorf("building request: %w", err)
	}
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("User-Agent", "AlphOne-Webhook/1")
	request.Header.Set("X-AlphOne-Event", string(d.EventName))
	request.Header.Set("X-AlphOne-Delivery", d.ID.String())
	request.Header.Set("X-AlphOne-Signature-256", Sign(d.Secret, d.Payload))
	response, err := w.client.Do(request)
	if err != nil {
		return fmt.Errorf("posting delivery: %w", err)
	}
	defer func() { _ = response.Body.Close() }()
	if response.StatusCode < 200 || response.StatusCode > 299 {
		return fmt.Errorf("subscriber answered %d", response.StatusCode)
	}
	return nil
}

// settle records the outcome of one delivery attempt.
func (w *Worker) settle(ctx context.Context, d ClaimedDelivery, status string, after time.Time, lastError string) {
	if err := w.queue.SettleDelivery(ctx, d.ID, status, after, lastError); err != nil {
		w.logger.ErrorContext(ctx, "settling webhook delivery", "delivery", d.ID, "error", err)
	}
}
