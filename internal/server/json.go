// SPDX-License-Identifier: Elastic-2.0

package server

import (
	"errors"
	"net/http"

	"github.com/gopherium/gouncer/authkit"

	"github.com/gopherium/alphone/internal/contact"
	"github.com/gopherium/alphone/internal/event"
	"github.com/gopherium/alphone/internal/task"
	"github.com/gopherium/alphone/internal/webhook"
)

// respondDomainError maps a domain error to an HTTP status and writes it as a JSON error response,
// masking internal errors.
func respondDomainError(w http.ResponseWriter, err error) {
	status, message := statusFor(err)
	authkit.RespondError(w, status, message)
}

// statusFor returns the HTTP status code and client-facing message for the
// given domain error, keeping third-party library wording out of the API
// contract and masking unrecognized errors as internal ones.
func statusFor(err error) (int, string) {
	if status, ok := contactStatusFor(err); ok {
		return status, err.Error()
	}
	if status, ok := taskStatusFor(err); ok {
		return status, err.Error()
	}
	if status, ok := webhookStatusFor(err); ok {
		return status, err.Error()
	}
	if status, message, ok := authkit.StatusForAuthError(err); ok {
		return status, message
	}
	return http.StatusInternalServerError, "internal error"
}

// contactStatusFor maps contact domain errors to their HTTP status.
func contactStatusFor(err error) (int, bool) {
	switch {
	case errors.Is(err, contact.ErrEmptyName), errors.Is(err, contact.ErrEmptyChannel),
		errors.Is(err, contact.ErrEmptyIdentifier), errors.Is(err, errChannelNotWritable):
		return http.StatusUnprocessableEntity, true
	case errors.Is(err, contact.ErrNotFound), errors.Is(err, contact.ErrIdentityNotFound):
		return http.StatusNotFound, true
	}
	return 0, false
}

// taskStatusFor maps task domain errors to their HTTP status.
func taskStatusFor(err error) (int, bool) {
	switch {
	case errors.Is(err, task.ErrEmptyTitle), errors.Is(err, task.ErrInvalidPriority),
		errors.Is(err, task.ErrInvalidStatus):
		return http.StatusUnprocessableEntity, true
	case errors.Is(err, task.ErrNotFound):
		return http.StatusNotFound, true
	}
	return 0, false
}

// webhookStatusFor maps webhook domain errors to their HTTP status.
func webhookStatusFor(err error) (int, bool) {
	switch {
	case errors.Is(err, webhook.ErrInvalidURL), errors.Is(err, webhook.ErrNoEvents),
		errors.Is(err, event.ErrUnknownName):
		return http.StatusUnprocessableEntity, true
	case errors.Is(err, webhook.ErrNotFound):
		return http.StatusNotFound, true
	}
	return 0, false
}
