// SPDX-License-Identifier: Elastic-2.0

package webhook

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
)

type countingQueue struct {
	mu        sync.Mutex
	claims    int
	settleErr error
}

// ClaimDueDeliveries counts the sweep and returns no deliveries.
func (q *countingQueue) ClaimDueDeliveries(_ context.Context, _, _ time.Time, _ int) ([]ClaimedDelivery, error) {
	q.mu.Lock()
	defer q.mu.Unlock()
	q.claims++
	return nil, nil
}

// SettleDelivery returns the configured settle failure.
func (q *countingQueue) SettleDelivery(_ context.Context, _ uuid.UUID, _ string, _ time.Time, _ string) error {
	return q.settleErr
}

// sweeps returns how many times the queue was claimed from.
func (q *countingQueue) sweeps() int {
	q.mu.Lock()
	defer q.mu.Unlock()
	return q.claims
}

func TestStoppingANeverStartedWorkerIsFine(t *testing.T) {
	t.Parallel()

	NewWorker(&countingQueue{}, slog.New(slog.NewTextHandler(&strings.Builder{}, nil))).Stop()
}

func TestWorkerSweepsOnEveryTick(t *testing.T) {
	t.Parallel()

	queue := &countingQueue{}
	worker := NewWorker(queue, slog.New(slog.NewTextHandler(&strings.Builder{}, nil)))
	worker.interval = time.Millisecond

	worker.Start()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) && queue.sweeps() < 3 {
		time.Sleep(time.Millisecond)
	}
	worker.Stop()

	if got := queue.sweeps(); got < 3 {
		t.Errorf("swept %d times, want the ticker driving repeated sweeps", got)
	}
}

func TestWorkerPostsOverItsOwnConnectionPool(t *testing.T) {
	t.Parallel()

	worker := NewWorker(&countingQueue{}, slog.New(slog.NewTextHandler(&strings.Builder{}, nil)))

	if worker.client.Transport == nil {
		t.Fatal("worker transport = nil, so it posts over the pool every other caller shares")
	}
	if worker.client.Transport == http.DefaultTransport {
		t.Error("worker shares http.DefaultTransport, so an idle connection closed elsewhere can break a delivery")
	}
}

func TestWorkerReportsAnUnusableSubscriberURL(t *testing.T) {
	t.Parallel()

	var logged strings.Builder
	worker := NewWorker(&countingQueue{}, slog.New(slog.NewTextHandler(&logged, nil)))

	err := worker.post(t.Context(), ClaimedDelivery{URL: "://not a url"})

	if err == nil {
		t.Error("post() error = nil, want the unusable url reported")
	}
}

func TestWorkerLogsAFailureToSettle(t *testing.T) {
	t.Parallel()

	var logged strings.Builder
	queue := &countingQueue{settleErr: errors.New("queue unavailable")}
	worker := NewWorker(queue, slog.New(slog.NewTextHandler(&logged, nil)))

	worker.settle(t.Context(), ClaimedDelivery{Delivery: Delivery{ID: uuid.Must(uuid.NewV7())}},
		StatusDelivered, time.Now().UTC(), "")

	if !strings.Contains(logged.String(), "level=ERROR") {
		t.Errorf("log = %q, want the failure recorded", logged.String())
	}
}
