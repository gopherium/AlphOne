// SPDX-License-Identifier: Elastic-2.0

package server

import (
	"net/http"
	"net/http/httptest"
	"strings"
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
