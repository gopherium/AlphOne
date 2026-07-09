// SPDX-License-Identifier: Elastic-2.0

package server

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestRespondReportsMarshalFailure(t *testing.T) {
	t.Parallel()

	recorder := httptest.NewRecorder()

	respond(recorder, http.StatusOK, make(chan int))

	if recorder.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusInternalServerError)
	}
	if !strings.Contains(recorder.Body.String(), "internal error") {
		t.Errorf("body = %q, want it to report an internal error", recorder.Body.String())
	}
}
