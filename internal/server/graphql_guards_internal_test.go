// SPDX-License-Identifier: Elastic-2.0

package server

import (
	"context"
	"io"
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

// arrivingBody is a request body that reports its first read and then waits until it is let go.
type arrivingBody struct {
	read    chan struct{}
	once    sync.Once
	letItGo chan struct{}
}

// Read reports the first read, then waits for letItGo and ends the body.
func (b *arrivingBody) Read([]byte) (int, error) {
	b.once.Do(func() { close(b.read) })
	<-b.letItGo
	return 0, io.EOF
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
	return withOperationGuards(blocked, operations, streams, DefaultGraphBounds), &started, func() { close(release) }
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

	bounds := GraphBounds{BodyMaxBytes: 2048, UploadMaxBytes: 4096, AnonymousBodyMaxBytes: 512}
	signedIn := uuid.Must(uuid.NewV7())

	jsonRequest := guardedRequest(signedIn)
	jsonRequest.Header.Set("Content-Type", "application/json")
	if got := graphBodyLimit(jsonRequest, bounds); got != 2048 {
		t.Errorf("signed in JSON budget = %d, want the configured 2048", got)
	}

	multipartRequest := guardedRequest(signedIn)
	multipartRequest.Header.Set("Content-Type", "multipart/form-data; boundary=upload")
	if got := graphBodyLimit(multipartRequest, bounds); got != 4096 {
		t.Errorf("signed in multipart budget = %d, want the configured 4096", got)
	}

	anonymousRequest := httptest.NewRequest(http.MethodPost, "/api/graphql", nil)
	anonymousRequest.Header.Set("Content-Type", "application/json")
	if got := graphBodyLimit(anonymousRequest, bounds); got != 512 {
		t.Errorf("anonymous JSON budget = %d, want the configured 512", got)
	}
}

// countedBody is a request body that counts the reads it serves.
type countedBody struct {
	reads int
}

// Read counts the read and ends the body.
func (b *countedBody) Read([]byte) (int, error) {
	b.reads++
	return 0, io.EOF
}

func TestAnonymousMultipartIsRefusedBeforeItsBodyOrASlot(t *testing.T) {
	t.Parallel()

	anonymous := newAddressLimiter(5, 20)
	operations := graphPolicy{
		limiter: newStreamLimiter(5), anonymous: anonymous, lifetime: time.Minute, overflow: overflowOperations,
	}
	streams := graphPolicy{
		limiter: newStreamLimiter(5), anonymous: anonymous, lifetime: time.Minute, overflow: overflowStreams,
	}
	guarded := withOperationGuards(noContent(), operations, streams, DefaultGraphBounds)
	body := &countedBody{}
	request := httptest.NewRequest(http.MethodPost, "/api/graphql", body)
	request.Header.Set("Content-Type", "multipart/form-data; boundary=upload")
	recorder := httptest.NewRecorder()

	guarded.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusUnauthorized || !strings.Contains(recorder.Body.String(), `"session_absent"`) {
		t.Errorf("anonymous multipart answered %d %s, want 401 session_absent", recorder.Code, recorder.Body.String())
	}
	if body.reads != 0 {
		t.Errorf("the refused body was read %d times, want it left unread", body.reads)
	}
	if anonymous.total != 0 {
		t.Errorf("the refused request held %d slots, want none", anonymous.total)
	}
}

func TestOperationGuardsTakeNoSlotWhileTheBodyIsStillArriving(t *testing.T) {
	t.Parallel()

	oneAnonymousSlot := newAddressLimiter(1, 1)
	operations := graphPolicy{
		limiter: newStreamLimiter(1), anonymous: oneAnonymousSlot, lifetime: time.Minute, overflow: overflowOperations,
	}
	streams := graphPolicy{
		limiter: newStreamLimiter(1), anonymous: oneAnonymousSlot, lifetime: time.Minute, overflow: overflowStreams,
	}
	readingTheBody := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.ReadAll(r.Body)
		w.WriteHeader(http.StatusNoContent)
	})
	guarded := withOperationGuards(readingTheBody, operations, streams, DefaultGraphBounds)
	arriving := &arrivingBody{read: make(chan struct{}), letItGo: make(chan struct{})}
	slow := httptest.NewRequest(http.MethodPost, "/api/graphql", arriving)
	slow.Header.Set("Content-Type", "application/json")
	slowDone := make(chan struct{})
	go func() {
		guarded.ServeHTTP(httptest.NewRecorder(), slow)
		close(slowDone)
	}()
	select {
	case <-arriving.read:
	case <-time.After(5 * time.Second):
		t.Fatal("the slow request never started reading its body")
	}

	ready := httptest.NewRequest(http.MethodPost, "/api/graphql", strings.NewReader(`{"query":"{ version }"}`))
	ready.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()
	guarded.ServeHTTP(recorder, ready)
	close(arriving.letItGo)
	<-slowDone

	if recorder.Code != http.StatusNoContent {
		t.Errorf("a ready request behind a body still arriving got %d, want 204 from the one slot", recorder.Code)
	}
}

func TestGraphBoundsFallBackToTheirDefaults(t *testing.T) {
	t.Parallel()

	if got := (GraphBounds{}).withDefaults(); got != DefaultGraphBounds {
		t.Errorf("GraphBounds{}.withDefaults() = %+v, want %+v", got, DefaultGraphBounds)
	}

	if DefaultGraphBounds.AnonymousBodyMaxBytes == 0 {
		t.Error("DefaultGraphBounds names no anonymous body limit, want one below the JSON limit")
	}

	named := GraphBounds{OperationsPerUser: 7}.withDefaults()
	want := DefaultGraphBounds
	want.OperationsPerUser = 7
	if named != want {
		t.Errorf("a bound named alone gave %+v, want %+v with the rest defaulted", named, want)
	}
}

func TestGraphPoliciesFollowTheBounds(t *testing.T) {
	t.Parallel()

	bounds := GraphBounds{OperationsPerUser: 7, OperationTimeout: 45 * time.Second, RetryAfter: 3 * time.Second}

	operations, _ := graphPolicies(bounds, 90*time.Second, 3)

	if operations.limiter.limit != 7 || operations.lifetime != 45*time.Second || operations.retryAfter != 3*time.Second {
		t.Errorf("operation policy = (%d, %v, %v), want (7, 45s, 3s)",
			operations.limiter.limit, operations.lifetime, operations.retryAfter)
	}
}

// acceptHeaders maps an Accept header onto whether it names a stream and no JSON answer.
var acceptHeaders = map[string]bool{
	"text/event-stream":                  true,
	"text/event-stream, multipart/mixed": true,
	"application/graphql-response+json, application/graphql+json, application/json, " +
		"text/event-stream, multipart/mixed": false,
	"text/event-stream, application/json":                  false,
	"application/graphql-response+json, text/event-stream": false,
	"application/graphql+json, text/event-stream":          false,
	"application/graphql-response+json, application/json":  false,
	"*/*": false,
	"":    false,
}

func TestAcceptsEventStreamRejectsACallerTakingJSON(t *testing.T) {
	t.Parallel()

	for accept, want := range acceptHeaders {
		request := httptest.NewRequest(http.MethodPost, "/api/graphql", nil)
		request.Header.Set("Content-Type", "application/json")
		request.Header.Set("Accept", accept)
		if got := acceptsEventStream(request); got != want {
			t.Errorf("Accept %q classified as stream = %t, want %t", accept, got, want)
		}
	}
}

func TestSubscriptionSSEMatchesTheStreamClassifier(t *testing.T) {
	t.Parallel()

	for accept, want := range acceptHeaders {
		request := httptest.NewRequest(http.MethodPost, "/api/graphql", nil)
		request.Header.Set("Content-Type", "application/json")
		request.Header.Set("Accept", accept)
		if got := (subscriptionSSE{}).Supports(request); got != want {
			t.Errorf("Accept %q served over the stream transport = %t, want %t", accept, got, want)
		}
	}
}

func TestSubscriptionSSENarrowsTheStockTransport(t *testing.T) {
	t.Parallel()

	request := httptest.NewRequest(http.MethodPost, "/api/graphql", nil)
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Accept", "application/json, text/event-stream")

	if !(transport.SSE{}).Supports(request) {
		t.Fatal("the stock transport no longer claims a caller offering JSON, so this narrowing is stale")
	}
	if (subscriptionSSE{}).Supports(request) {
		t.Error("a caller offering JSON is served an event stream, so its queries spend the stream budget")
	}
}

func TestGraphOperationBudgetFitsAScreenLoad(t *testing.T) {
	t.Parallel()

	const screenLoad = 8
	operations, _ := graphPolicies(DefaultGraphBounds, time.Minute, DefaultMaxStreamsPerUser)
	user := uuid.Must(uuid.NewV7())

	for i := range screenLoad {
		if !operations.limiter.acquire(user) {
			t.Fatalf("operation %d of a screen load was refused, want a budget fitting %d", i+1, screenLoad)
		}
	}
}

func TestRetryAfterSecondsRoundsUpToAWholeSecond(t *testing.T) {
	t.Parallel()

	tests := map[time.Duration]int{
		0:                       1,
		time.Millisecond:        1,
		time.Second:             1,
		1500 * time.Millisecond: 2,
		90 * time.Second:        90,
		5 * time.Minute:         300,
	}

	for given, want := range tests {
		if got := retryAfterSeconds(given); got != want {
			t.Errorf("retryAfterSeconds(%v) = %d, want %d", given, got, want)
		}
	}
}

func TestGraphPoliciesHintARetryMatchingHowSoonASlotFrees(t *testing.T) {
	t.Parallel()

	operations, streams := graphPolicies(DefaultGraphBounds, 90*time.Second, 3)

	if operations.retryAfter != DefaultGraphBounds.RetryAfter {
		t.Errorf("operation retry hint = %v, want %v", operations.retryAfter, DefaultGraphBounds.RetryAfter)
	}
	if operations.retryAfter >= operations.lifetime {
		t.Errorf("operation retry hint = %v, want well under the %v one operation may hold a slot",
			operations.retryAfter, operations.lifetime)
	}
	if streams.retryAfter != 90*time.Second {
		t.Errorf("stream retry hint = %v, want the %v a stream may hold a slot", streams.retryAfter, 90*time.Second)
	}
}

func TestOperationGuardsHintTheRetryOfTheSpentBudget(t *testing.T) {
	t.Parallel()

	operations := graphPolicy{
		limiter:    newStreamLimiter(0),
		lifetime:   time.Minute,
		retryAfter: 2 * time.Second,
		overflow:   overflowOperations,
	}
	streams := graphPolicy{
		limiter:    newStreamLimiter(0),
		lifetime:   time.Hour,
		retryAfter: 90 * time.Second,
		overflow:   overflowStreams,
	}
	guarded := withOperationGuards(noContent(), operations, streams, DefaultGraphBounds)
	user := uuid.Must(uuid.NewV7())

	refusedOperation := httptest.NewRecorder()
	guarded.ServeHTTP(refusedOperation, guardedRequest(user))
	if got := refusedOperation.Header().Get("Retry-After"); got != "2" {
		t.Errorf("operation Retry-After = %q, want %q rather than its lifetime", got, "2")
	}

	refusedStream := httptest.NewRecorder()
	guarded.ServeHTTP(refusedStream, streamingRequest(user))
	if got := refusedStream.Header().Get("Retry-After"); got != "90" {
		t.Errorf("stream Retry-After = %q, want %q rather than its lifetime", got, "90")
	}
}

func TestGraphPoliciesGiveStreamsTheHostBounds(t *testing.T) {
	t.Parallel()

	operations, streams := graphPolicies(DefaultGraphBounds, 90*time.Second, 3)

	if operations.lifetime != DefaultGraphBounds.OperationTimeout ||
		operations.limiter.limit != DefaultGraphBounds.OperationsPerUser {
		t.Errorf("operation policy = (%v, %d), want (%v, %d)", operations.lifetime, operations.limiter.limit,
			DefaultGraphBounds.OperationTimeout, DefaultGraphBounds.OperationsPerUser)
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

	passing := withOperationGuards(noContent(), operations, streams, DefaultGraphBounds)
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

	passing := withOperationGuards(noContent(), operations, streams, DefaultGraphBounds)
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
	guarded := withOperationGuards(waiting, operations, streams, DefaultGraphBounds)

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
	guarded := withOperationGuards(waiting, operations, streams, DefaultGraphBounds)

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
