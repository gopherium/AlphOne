// SPDX-License-Identifier: Elastic-2.0

package server

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/gopherium/gouncer/authkit"
)

// identityRequest builds a request stamped with the acting user.
func guardedRequest(userID uuid.UUID) *http.Request {
	request := httptest.NewRequest(http.MethodPost, "/api/graphql", nil)
	ctx := authkit.WithIdentity(request.Context(), authkit.Identity{ID: userID})
	return request.WithContext(ctx)
}

func TestOperationGuardsRejectTheSixthConcurrentOperation(t *testing.T) {
	t.Parallel()

	release := make(chan struct{})
	var started sync.WaitGroup
	blocked := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		started.Done()
		<-release
		w.WriteHeader(http.StatusNoContent)
	})
	limiter := newStreamLimiter(5)
	guarded := withOperationGuards(blocked, limiter, time.Minute)
	user := uuid.Must(uuid.NewV7())

	var inFlight sync.WaitGroup
	started.Add(5)
	for range 5 {
		inFlight.Add(1)
		go func() {
			defer inFlight.Done()
			guarded.ServeHTTP(httptest.NewRecorder(), guardedRequest(user))
		}()
	}
	started.Wait()

	rejected := httptest.NewRecorder()
	guarded.ServeHTTP(rejected, guardedRequest(user))
	if rejected.Code != http.StatusTooManyRequests {
		t.Errorf("sixth operation = %d, want %d", rejected.Code, http.StatusTooManyRequests)
	}

	close(release)
	inFlight.Wait()

	unblocked := withOperationGuards(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}), limiter, time.Minute)
	afterRelease := httptest.NewRecorder()
	unblocked.ServeHTTP(afterRelease, guardedRequest(user))
	if afterRelease.Code != http.StatusNoContent {
		t.Errorf("operation after release = %d, want %d", afterRelease.Code, http.StatusNoContent)
	}
}

func TestOperationGuardsDeadlineTheRequestContext(t *testing.T) {
	t.Parallel()

	sawDeadline := make(chan error, 1)
	waiting := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		<-r.Context().Done()
		sawDeadline <- r.Context().Err()
		w.WriteHeader(http.StatusNoContent)
	})
	guarded := withOperationGuards(waiting, newStreamLimiter(5), 20*time.Millisecond)

	start := time.Now()
	guarded.ServeHTTP(httptest.NewRecorder(), guardedRequest(uuid.Must(uuid.NewV7())))

	if elapsed := time.Since(start); elapsed > 2*time.Second {
		t.Fatalf("request lasted %v, want the 20ms deadline to cut it", elapsed)
	}
	if err := <-sawDeadline; err != context.DeadlineExceeded {
		t.Errorf("context error = %v, want DeadlineExceeded", err)
	}
}
