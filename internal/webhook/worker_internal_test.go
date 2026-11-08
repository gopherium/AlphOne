// SPDX-License-Identifier: Elastic-2.0

package webhook

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"slices"
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
	limits    []int
	// answers holds how many payloads each clear empties in turn, none once it runs out.
	answers []int
}

// ClearSettledPayloads records the limit of one clear and answers the next of answers.
func (q *countingQueue) ClearSettledPayloads(_ context.Context, _ time.Time, limit int) (int, error) {
	q.mu.Lock()
	defer q.mu.Unlock()
	q.limits = append(q.limits, limit)
	if len(q.answers) == 0 {
		return 0, nil
	}
	cleared := q.answers[0]
	q.answers = q.answers[1:]
	return cleared, nil
}

// clearLimits returns the limit of every clear so far.
func (q *countingQueue) clearLimits() []int {
	q.mu.Lock()
	defer q.mu.Unlock()
	return slices.Clone(q.limits)
}

// ClaimDueDeliveries counts the sweep and returns no deliveries.
func (q *countingQueue) ClaimDueDeliveries(_ context.Context, _, _ time.Time, _ int) ([]ClaimedDelivery, error) {
	q.mu.Lock()
	defer q.mu.Unlock()
	q.claims++
	return nil, nil
}

// SettleDelivery returns the configured settlement error.
func (q *countingQueue) SettleDelivery(_ context.Context, _ uuid.UUID, _ string, _ time.Time, _ string) error {
	return q.settleErr
}

// OwnerDisabled reports every owner enabled.
func (q *countingQueue) OwnerDisabled(context.Context, uuid.UUID) (bool, error) {
	return false, nil
}

// sweeps returns how many times the queue was claimed from.
func (q *countingQueue) sweeps() int {
	q.mu.Lock()
	defer q.mu.Unlock()
	return q.claims
}

// sweptAndClearedAtLeast reports whether the queue was claimed from and cleared at least times each.
func (q *countingQueue) sweptAndClearedAtLeast(times int) bool {
	q.mu.Lock()
	defer q.mu.Unlock()
	return q.claims >= times && len(q.limits) >= times
}

func TestStoppingANeverStartedWorkerIsFine(t *testing.T) {
	t.Parallel()

	NewWorker(&countingQueue{}, slog.New(slog.NewTextHandler(&strings.Builder{}, nil)), AddressGuard{},
		DefaultPayloadAge).Stop()
}

func TestWorkerClearsAtMostAsManyPayloadsAsOneSweepClaims(t *testing.T) {
	t.Parallel()

	queue := &countingQueue{}

	NewWorker(queue, slog.New(slog.DiscardHandler), AddressGuard{}, DefaultPayloadAge).ClearPayloads(t.Context())

	if got := queue.clearLimits(); !slices.Equal(got, []int{sweepBatch}) {
		t.Errorf("clear limits = %v, want one clear of at most %d payloads", got, sweepBatch)
	}
}

func TestClearingGoesOnUntilAClearComesBackShortOfAFullBatch(t *testing.T) {
	t.Parallel()

	queue := &countingQueue{answers: []int{sweepBatch, sweepBatch - 1, sweepBatch}}

	NewWorker(queue, slog.New(slog.DiscardHandler), AddressGuard{}, DefaultPayloadAge).ClearPayloads(t.Context())

	if got := queue.clearLimits(); !slices.Equal(got, []int{sweepBatch, sweepBatch}) {
		t.Errorf("clear limits = %v, want a full clear of %d and then a short one", got, sweepBatch)
	}
}

func TestClearingStopsOnceItsContextEnds(t *testing.T) {
	t.Parallel()

	queue := &countingQueue{answers: []int{sweepBatch, sweepBatch, sweepBatch}}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	NewWorker(queue, slog.New(slog.DiscardHandler), AddressGuard{}, DefaultPayloadAge).ClearPayloads(ctx)

	if got := queue.clearLimits(); len(got) != 1 {
		t.Errorf("clear limits = %v, want one clear once the context ended", got)
	}
}

func TestWorkerSweepsAndClearsOnEveryTick(t *testing.T) {
	t.Parallel()

	queue := &countingQueue{}
	worker := NewWorker(queue, slog.New(slog.NewTextHandler(&strings.Builder{}, nil)), AddressGuard{}, DefaultPayloadAge)
	worker.interval = time.Millisecond

	worker.Start()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) && !queue.sweptAndClearedAtLeast(3) {
		time.Sleep(time.Millisecond)
	}
	worker.Stop()

	if got := queue.sweeps(); got < 3 {
		t.Errorf("swept %d times, want the ticker driving repeated sweeps", got)
	}
	if got := len(queue.clearLimits()); got < 3 {
		t.Errorf("cleared %d times, want the ticker driving repeated clears", got)
	}
}

func TestWorkerPostsOverItsOwnConnectionPool(t *testing.T) {
	t.Parallel()

	worker := NewWorker(&countingQueue{}, slog.New(slog.NewTextHandler(&strings.Builder{}, nil)), AddressGuard{},
		DefaultPayloadAge)

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
	worker := NewWorker(&countingQueue{}, slog.New(slog.NewTextHandler(&logged, nil)), AddressGuard{}, DefaultPayloadAge)

	err := worker.post(t.Context(), ClaimedDelivery{URL: "://not a url"})

	if err == nil {
		t.Error("post() error = nil, want the unusable url reported")
	}
}

func TestWorkerLogsAFailureToSettle(t *testing.T) {
	t.Parallel()

	var logged strings.Builder
	queue := &countingQueue{settleErr: errors.New("queue unavailable")}
	worker := NewWorker(queue, slog.New(slog.NewTextHandler(&logged, nil)), AddressGuard{}, DefaultPayloadAge)

	worker.settle(t.Context(), ClaimedDelivery{Delivery: Delivery{ID: uuid.Must(uuid.NewV7())}},
		StatusDelivered, time.Now().UTC(), "")

	if !strings.Contains(logged.String(), "level=ERROR") {
		t.Errorf("log = %q, want the failure recorded", logged.String())
	}
}
