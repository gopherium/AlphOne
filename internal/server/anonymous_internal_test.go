// SPDX-License-Identifier: Elastic-2.0

package server

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
)

func TestAnonymousCallersFromTwoAddressesDoNotShareSlots(t *testing.T) {
	t.Parallel()

	slots := newAddressLimiter(2, 20)
	for i := range 2 {
		if !slots.acquire("198.51.100.1") {
			t.Fatalf("slot %d of the first address was refused, want two", i+1)
		}
	}

	if !slots.acquire("198.51.100.2") {
		t.Error("the second address was refused while the first held its own slots, want a pool each")
	}
}

func TestOneAnonymousAddressStopsAtItsCap(t *testing.T) {
	t.Parallel()

	slots := newAddressLimiter(3, 20)
	for i := range 3 {
		if !slots.acquire("198.51.100.1") {
			t.Fatalf("slot %d of the address was refused, want three", i+1)
		}
	}

	if slots.acquire("198.51.100.1") {
		t.Error("a fourth slot was granted to one address, want it held to its cap of three")
	}
}

func TestAnonymousCeilingHoldsAcrossManyAddresses(t *testing.T) {
	t.Parallel()

	slots := newAddressLimiter(5, 4)
	for _, address := range []string{"198.51.100.1", "198.51.100.2", "198.51.100.3", "198.51.100.4"} {
		if !slots.acquire(address) {
			t.Fatalf("%s was refused under the ceiling, want it admitted", address)
		}
	}

	if slots.acquire("198.51.100.5") {
		t.Error("a fifth address was admitted past a ceiling of four, want the ceiling to hold")
	}
}

func TestFreedAnonymousAddressLeavesNoEntry(t *testing.T) {
	t.Parallel()

	slots := newAddressLimiter(5, 20)
	slots.acquire("198.51.100.1")

	slots.release("198.51.100.1")

	if len(slots.counts) != 0 || slots.total != 0 {
		t.Errorf("after the only slot freed the limiter holds %v and %d in total, want nothing", slots.counts, slots.total)
	}
}

// anonymousFrom builds a locale request carrying no identity from the address.
func anonymousFrom(remoteAddr string) *http.Request {
	request := httptest.NewRequest(http.MethodPost, "/api/graphql", strings.NewReader(`{"query":"{ locale }"}`))
	request.Header.Set("Content-Type", "application/json")
	request.RemoteAddr = remoteAddr
	return request
}

// heldAddress is the client address whose request the holding guard keeps in flight.
const heldAddress = "198.51.100.1:40000"

// holdingGuard returns a guard keeping the request from heldAddress in flight until let go, answering others at once.
func holdingGuard(bounds GraphBounds) (http.Handler, <-chan struct{}, func()) {
	operations, streams := graphPolicies(bounds.withDefaults(), time.Minute, 5)
	held, letGo := make(chan struct{}), make(chan struct{})
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.RemoteAddr == heldAddress {
			close(held)
			<-letGo
		}
		w.WriteHeader(http.StatusNoContent)
	})
	return withOperationGuards(handler, operations, streams, DefaultGraphBounds), held, func() { close(letGo) }
}

// holdOne sends the held request and returns once it holds its slot.
func holdOne(t *testing.T, guarded http.Handler, held <-chan struct{}) {
	t.Helper()
	go guarded.ServeHTTP(httptest.NewRecorder(), anonymousFrom(heldAddress))
	select {
	case <-held:
	case <-time.After(5 * time.Second):
		t.Fatal("the held request never reached the handler")
	}
}

func TestAnonymousOperationsAreKeyedByTheirAddress(t *testing.T) {
	t.Parallel()

	guarded, held, letGo := holdingGuard(GraphBounds{AnonymousPerIP: 1})
	defer letGo()
	holdOne(t, guarded, held)

	sameAddress := httptest.NewRecorder()
	guarded.ServeHTTP(sameAddress, anonymousFrom("198.51.100.1:40001"))
	otherAddress := httptest.NewRecorder()
	guarded.ServeHTTP(otherAddress, anonymousFrom("198.51.100.2:40000"))

	if sameAddress.Code != http.StatusTooManyRequests {
		t.Errorf("a second request from the held address answered %d, want 429", sameAddress.Code)
	}
	if otherAddress.Code != http.StatusNoContent {
		t.Errorf("a request from another address answered %d, want 204 from its own pool", otherAddress.Code)
	}
}

func TestSignedInCallersKeepTheirOwnPool(t *testing.T) {
	t.Parallel()

	guarded, held, letGo := holdingGuard(GraphBounds{AnonymousCeiling: 1})
	defer letGo()
	holdOne(t, guarded, held)

	anonymous := httptest.NewRecorder()
	guarded.ServeHTTP(anonymous, anonymousFrom("198.51.100.2:40000"))
	signedIn := httptest.NewRecorder()
	guarded.ServeHTTP(signedIn, guardedRequest(uuid.Must(uuid.NewV7())))

	if anonymous.Code != http.StatusTooManyRequests {
		t.Errorf("an anonymous request past the ceiling of one answered %d, want 429", anonymous.Code)
	}
	if signedIn.Code != http.StatusNoContent {
		t.Errorf("a signed in request under a full anonymous ceiling answered %d, want 204 from its own pool", signedIn.Code)
	}
}

// stalledWriter is a response writer whose first write waits until it is let go.
type stalledWriter struct {
	header  http.Header
	writing chan struct{}
	once    sync.Once
	letGo   chan struct{}
}

// Header returns the header of the stalled answer.
func (s *stalledWriter) Header() http.Header {
	return s.header
}

// WriteHeader takes the status of the stalled answer.
func (s *stalledWriter) WriteHeader(int) {}

// Write reports the first write, then waits until the writer is let go.
func (s *stalledWriter) Write(p []byte) (int, error) {
	s.once.Do(func() { close(s.writing) })
	<-s.letGo
	return len(p), nil
}

func TestAnAnonymousSlotFreesBeforeItsAnswerIsWritten(t *testing.T) {
	t.Parallel()

	bounds := GraphBounds{AnonymousPerIP: 1}.withDefaults()
	operations, streams := graphPolicies(bounds, time.Minute, 5)
	answering := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"data":{"locale":"en-US"}}`))
	})
	guarded := withOperationGuards(answering, operations, streams, bounds)
	stalled := &stalledWriter{header: http.Header{}, writing: make(chan struct{}), letGo: make(chan struct{})}
	answered := make(chan struct{})
	go func() {
		guarded.ServeHTTP(stalled, anonymousFrom(heldAddress))
		close(answered)
	}()
	select {
	case <-stalled.writing:
	case <-time.After(5 * time.Second):
		t.Fatal("the answer never reached the caller")
	}

	freed := operations.anonymous.acquire("198.51.100.1")
	close(stalled.letGo)
	<-answered

	if !freed {
		t.Error("the address held its only slot while its answer waited on the caller, want the slot freed first")
	}
}

// sizedAnswer answers with a cookie, a 422 and a body of size bytes written in two halves.
func sizedAnswer(size int) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.SetCookie(w, &http.Cookie{Name: "held", Value: "1"})
		w.WriteHeader(http.StatusUnprocessableEntity)
		_, _ = w.Write([]byte(strings.Repeat("x", size/2)))
		_, _ = w.Write([]byte(strings.Repeat("x", size-size/2)))
	})
}

func TestAnAnonymousAnswerAtItsLimitReachesTheCallerWhole(t *testing.T) {
	t.Parallel()

	bounds := GraphBounds{AnonymousAnswerMaxBytes: 64}.withDefaults()
	operations, streams := graphPolicies(bounds, time.Minute, 5)
	recorder := httptest.NewRecorder()

	withOperationGuards(sizedAnswer(64), operations, streams, bounds).ServeHTTP(recorder, anonymousFrom(heldAddress))

	if recorder.Code != http.StatusUnprocessableEntity || recorder.Body.String() != strings.Repeat("x", 64) ||
		recorder.Header().Get("Set-Cookie") == "" {
		t.Errorf("an answer at its limit reached the caller as %d %q with cookie %q, want its 422, 64 bytes and cookie",
			recorder.Code, recorder.Body.String(), recorder.Header().Get("Set-Cookie"))
	}
}

func TestAnAnonymousAnswerOverItsLimitIsReplacedByTheRefusal(t *testing.T) {
	t.Parallel()

	bounds := GraphBounds{AnonymousAnswerMaxBytes: 64}.withDefaults()
	operations, streams := graphPolicies(bounds, time.Minute, 5)
	recorder := httptest.NewRecorder()

	withOperationGuards(sizedAnswer(65), operations, streams, bounds).ServeHTTP(recorder, anonymousFrom(heldAddress))

	answer := recorder.Body.String()
	if recorder.Code != http.StatusOK || !strings.Contains(answer, unauthenticatedAnswer) ||
		strings.Contains(answer, "xxx") || recorder.Header().Get("Set-Cookie") != "" {
		t.Errorf("an answer past its limit reached the caller as %d %q with cookie %q, want only 200 UNAUTHENTICATED",
			recorder.Code, answer, recorder.Header().Get("Set-Cookie"))
	}
}

func TestAnAnonymousStreamRequestRunsOnTheOperationLifetime(t *testing.T) {
	t.Parallel()

	bounds := GraphBounds{OperationTimeout: time.Second}.withDefaults()
	operations, streams := graphPolicies(bounds, time.Hour, 5)
	var left time.Duration
	measuring := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if deadline, held := r.Context().Deadline(); held {
			left = time.Until(deadline)
		}
		w.WriteHeader(http.StatusNoContent)
	})
	request := anonymousFrom(heldAddress)
	request.Header.Set("Accept", "text/event-stream")

	withOperationGuards(measuring, operations, streams, bounds).ServeHTTP(httptest.NewRecorder(), request)

	if left <= 0 || left > time.Second {
		t.Errorf("an anonymous stream request ran with %v left, want the operation lifetime of 1s", left)
	}
}
