// SPDX-License-Identifier: Elastic-2.0

package whatsapp

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gopherium/alphone/sdk"
)

func TestRespondSendErrorDefaultsToInternal(t *testing.T) {
	t.Parallel()

	recorder := httptest.NewRecorder()

	respondSendError(recorder, sdk.GraphError{Code: "VALIDATION", Err: errors.New("blank content")})

	if recorder.Code != http.StatusInternalServerError {
		t.Errorf("status = %d, want %d", recorder.Code, http.StatusInternalServerError)
	}
}
