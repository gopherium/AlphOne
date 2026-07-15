// SPDX-License-Identifier: Elastic-2.0

package server

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gopherium/gouncer"

	"github.com/gopherium/alphone/internal/contact"
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

func TestStatusForMapsDomainErrors(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		err         error
		wantStatus  int
		wantMessage string
	}{
		{"empty contact name", contact.ErrEmptyName, http.StatusUnprocessableEntity, contact.ErrEmptyName.Error()},
		{"invalid email", gouncer.ErrInvalidEmail, http.StatusUnprocessableEntity, "invalid email address"},
		{"empty user name", gouncer.ErrEmptyName, http.StatusUnprocessableEntity, "name is required"},
		{"name too long", gouncer.ErrNameTooLong, http.StatusUnprocessableEntity, "name must be at most 256 characters"},
		{"weak password", gouncer.ErrWeakPassword, http.StatusUnprocessableEntity, "password must be at least 12 characters"},
		{
			"password too long",
			gouncer.ErrPasswordTooLong,
			http.StatusUnprocessableEntity,
			"password must be at most 1024 characters",
		},
		{"contact not found", contact.ErrNotFound, http.StatusNotFound, contact.ErrNotFound.Error()},
		{"user not found", gouncer.ErrUserNotFound, http.StatusNotFound, "user not found"},
		{"email taken", gouncer.ErrEmailTaken, http.StatusConflict, "email already in use"},
		{"unrecognized error", errors.New("boom"), http.StatusInternalServerError, "internal error"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			status, message := statusFor(tt.err)

			if status != tt.wantStatus {
				t.Errorf("statusFor(%v) status = %d, want %d", tt.err, status, tt.wantStatus)
			}
			if message != tt.wantMessage {
				t.Errorf("statusFor(%v) message = %q, want %q", tt.err, message, tt.wantMessage)
			}
		})
	}
}
