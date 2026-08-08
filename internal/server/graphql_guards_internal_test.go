// SPDX-License-Identifier: Elastic-2.0

package server

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/99designs/gqlgen/graphql/handler/transport"
	"github.com/google/uuid"

	"github.com/gopherium/gouncer/authkit"
)

// guardedRequest builds a request stamped with the acting user.
func guardedRequest(userID uuid.UUID) *http.Request {
	request := httptest.NewRequest(http.MethodPost, "/api/graphql", nil)
	ctx := authkit.WithIdentity(request.Context(), authkit.Identity{ID: userID})
	return request.WithContext(ctx)
}

// streamingRequest builds a request asking for a Server-Sent Events response.
func streamingRequest(userID uuid.UUID) *http.Request {
	request := guardedRequest(userID)
	request.Header.Set("Accept", "text/event-stream")
	return request
}

// noContent answers every request with 204.
func noContent() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})
}

// testPolicies returns an operation and a stream budget of five slots each.
func testPolicies() (graphPolicy, graphPolicy) {
	return graphPolicy{limiter: newStreamLimiter(5), lifetime: time.Minute, overflow: overflowOperations},
		graphPolicy{limiter: newStreamLimiter(5), lifetime: time.Minute, overflow: overflowStreams}
}

// blockingGuard returns a guard holding every request until the returned
// release runs, beside the group counting requests that reached the handler.
func blockingGuard(operations, streams graphPolicy) (http.Handler, *sync.WaitGroup, func()) {
	release := make(chan struct{})
	var started sync.WaitGroup
	blocked := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		started.Done()
		<-release
		w.WriteHeader(http.StatusNoContent)
	})
	return withOperationGuards(blocked, operations, streams), &started, func() { close(release) }
}

// fillBudget holds five requests of one kind in flight, returning their group.
func fillBudget(
	t *testing.T, guarded http.Handler, started *sync.WaitGroup, request func() *http.Request,
) *sync.WaitGroup {
	t.Helper()
	var inFlight sync.WaitGroup
	started.Add(5)
	for range 5 {
		inFlight.Add(1)
		go func() {
			defer inFlight.Done()
			guarded.ServeHTTP(httptest.NewRecorder(), request())
		}()
	}
	arrived := make(chan struct{})
	go func() {
		started.Wait()
		close(arrived)
	}()
	select {
	case <-arrived:
	case <-time.After(5 * time.Second):
		t.Fatal("fewer than five requests reached the handler, want the budget filled")
	}
	return &inFlight
}

func TestGraphBodyLimitSplitsByContentType(t *testing.T) {
	t.Parallel()

	jsonRequest := httptest.NewRequest(http.MethodPost, "/api/graphql", nil)
	jsonRequest.Header.Set("Content-Type", "application/json")
	if got := graphBodyLimit(jsonRequest); got != graphJSONBodyLimit {
		t.Errorf("JSON budget = %d, want %d", got, graphJSONBodyLimit)
	}

	multipartRequest := httptest.NewRequest(http.MethodPost, "/api/graphql", nil)
	multipartRequest.Header.Set("Content-Type", "multipart/form-data; boundary=upload")
	if got := graphBodyLimit(multipartRequest); got != graphMultipartBodyLimit {
		t.Errorf("multipart budget = %d, want %d", got, graphMultipartBodyLimit)
	}
}

func TestAcceptsEventStreamMatchesTheSSETransportOnAccept(t *testing.T) {
	t.Parallel()

	for _, accept := range []string{
		"text/event-stream",
		"text/event-stream, application/json",
		"application/graphql-response+json, application/json",
		"*/*",
		"",
	} {
		request := httptest.NewRequest(http.MethodPost, "/api/graphql", nil)
		request.Header.Set("Content-Type", "application/json")
		request.Header.Set("Accept", accept)
		if got, want := acceptsEventStream(request), (transport.SSE{}).Supports(request); got != want {
			t.Errorf("Accept %q classified as stream = %t, want the SSE transport's %t", accept, got, want)
		}
	}
}

func TestGraphPoliciesGiveStreamsTheHostBounds(t *testing.T) {
	t.Parallel()

	operations, streams := graphPolicies(90*time.Second, 3)

	if operations.lifetime != graphOperationTimeout || operations.limiter.limit != graphMaxConcurrentOps {
		t.Errorf("operation policy = (%v, %d), want (%v, %d)",
			operations.lifetime, operations.limiter.limit, graphOperationTimeout, graphMaxConcurrentOps)
	}
	if streams.lifetime != 90*time.Second || streams.limiter.limit != 3 {
		t.Errorf("stream policy = (%v, %d), want (1m30s, 3)", streams.lifetime, streams.limiter.limit)
	}
	if operations.limiter == streams.limiter {
		t.Error("both kinds share one limiter, want a budget each")
	}
	if operations.overflow == streams.overflow {
		t.Errorf("both kinds answer %q, want an answer each", operations.overflow)
	}
}

func TestOperationGuardsRejectTheSixthConcurrentOperation(t *testing.T) {
	t.Parallel()

	operations, streams := testPolicies()
	guarded, started, release := blockingGuard(operations, streams)
	user := uuid.Must(uuid.NewV7())
	inFlight := fillBudget(t, guarded, started, func() *http.Request { return guardedRequest(user) })

	rejected := httptest.NewRecorder()
	guarded.ServeHTTP(rejected, guardedRequest(user))
	if rejected.Code != http.StatusTooManyRequests {
		t.Errorf("sixth operation = %d, want %d", rejected.Code, http.StatusTooManyRequests)
	}
	if !strings.Contains(rejected.Body.String(), overflowOperations) {
		t.Errorf("sixth operation body = %s, want %q", rejected.Body, overflowOperations)
	}

	passing := withOperationGuards(noContent(), operations, streams)
	admitted := httptest.NewRecorder()
	passing.ServeHTTP(admitted, streamingRequest(user))
	if admitted.Code != http.StatusNoContent {
		t.Errorf("stream over a spent operation budget = %d, want %d", admitted.Code, http.StatusNoContent)
	}

	release()
	inFlight.Wait()

	afterRelease := httptest.NewRecorder()
	passing.ServeHTTP(afterRelease, guardedRequest(user))
	if afterRelease.Code != http.StatusNoContent {
		t.Errorf("operation after release = %d, want %d", afterRelease.Code, http.StatusNoContent)
	}
}

func TestOperationGuardsRejectTheSixthConcurrentStream(t *testing.T) {
	t.Parallel()

	operations, streams := testPolicies()
	guarded, started, release := blockingGuard(operations, streams)
	user := uuid.Must(uuid.NewV7())
	inFlight := fillBudget(t, guarded, started, func() *http.Request { return streamingRequest(user) })

	rejected := httptest.NewRecorder()
	guarded.ServeHTTP(rejected, streamingRequest(user))
	if rejected.Code != http.StatusTooManyRequests {
		t.Errorf("sixth stream = %d, want %d", rejected.Code, http.StatusTooManyRequests)
	}
	if !strings.Contains(rejected.Body.String(), overflowStreams) {
		t.Errorf("sixth stream body = %s, want %q", rejected.Body, overflowStreams)
	}

	passing := withOperationGuards(noContent(), operations, streams)
	admitted := httptest.NewRecorder()
	passing.ServeHTTP(admitted, guardedRequest(user))
	if admitted.Code != http.StatusNoContent {
		t.Errorf("operation over a spent stream budget = %d, want %d", admitted.Code, http.StatusNoContent)
	}

	release()
	inFlight.Wait()
}

func TestOperationGuardsDeadlineTheRequestContext(t *testing.T) {
	t.Parallel()

	sawDeadline := make(chan error, 1)
	waiting := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		<-r.Context().Done()
		sawDeadline <- r.Context().Err()
		w.WriteHeader(http.StatusNoContent)
	})
	operations, streams := testPolicies()
	operations.lifetime = 20 * time.Millisecond
	guarded := withOperationGuards(waiting, operations, streams)

	start := time.Now()
	guarded.ServeHTTP(httptest.NewRecorder(), guardedRequest(uuid.Must(uuid.NewV7())))

	if elapsed := time.Since(start); elapsed > 2*time.Second {
		t.Fatalf("request lasted %v, want the 20ms deadline to cut it", elapsed)
	}
	if err := <-sawDeadline; err != context.DeadlineExceeded {
		t.Errorf("context error = %v, want DeadlineExceeded", err)
	}
}

func TestOperationGuardsHoldAStreamToTheStreamLifetime(t *testing.T) {
	t.Parallel()

	sawDeadline := make(chan error, 1)
	waiting := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		<-r.Context().Done()
		sawDeadline <- r.Context().Err()
		w.WriteHeader(http.StatusNoContent)
	})
	operations, streams := testPolicies()
	operations.lifetime = 10 * time.Millisecond
	streams.lifetime = 150 * time.Millisecond
	guarded := withOperationGuards(waiting, operations, streams)

	start := time.Now()
	guarded.ServeHTTP(httptest.NewRecorder(), streamingRequest(uuid.Must(uuid.NewV7())))
	elapsed := time.Since(start)

	if elapsed < 100*time.Millisecond {
		t.Errorf("stream lasted %v, want it to outlive the 10ms operation deadline", elapsed)
	}
	if err := <-sawDeadline; err != context.DeadlineExceeded {
		t.Errorf("context error = %v, want the stream lifetime to cut it", err)
	}
}
