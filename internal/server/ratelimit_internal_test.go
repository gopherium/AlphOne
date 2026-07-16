// SPDX-License-Identifier: Elastic-2.0

package server

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

var errCounterDown = errors.New("counter store is down")

type failingLimitCounter struct{}

func (failingLimitCounter) Config(_ int, _ time.Duration) {}

func (failingLimitCounter) Increment(_ string, _ time.Time) error {
	return errCounterDown
}

func (failingLimitCounter) IncrementBy(_ string, _ time.Time, _ int) error {
	return errCounterDown
}

func (failingLimitCounter) Get(_ string, _, _ time.Time) (int, int, error) {
	return 0, 0, errCounterDown
}

func TestLoginRateLimiterFailsClosedWhenTheCounterErrors(t *testing.T) {
	t.Parallel()

	var handlerRan bool
	handler := loginRateLimiterUsing(failingLimitCounter{})(
		http.HandlerFunc(func(http.ResponseWriter, *http.Request) { handlerRan = true }))

	request := httptest.NewRequest(http.MethodPost, "/api/auth/login", strings.NewReader(`{}`))
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want %d when the limit counter fails", recorder.Code, http.StatusInternalServerError)
	}
	if handlerRan {
		t.Error("the login handler ran despite the limit counter failing")
	}
	var body struct {
		Error string `json:"error"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
		t.Fatalf("response body %q is not JSON: %v", recorder.Body.String(), err)
	}
	if body.Error == "" {
		t.Error("fail-closed response carries no JSON error message")
	}
}
