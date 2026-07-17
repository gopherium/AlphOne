// SPDX-License-Identifier: Elastic-2.0

package server

import (
	"errors"
	"net/http"

	"github.com/gopherium/gouncer/authkit"

	"github.com/gopherium/alphone/internal/contact"
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
	switch {
	case errors.Is(err, contact.ErrEmptyName):
		return http.StatusUnprocessableEntity, err.Error()
	case errors.Is(err, contact.ErrNotFound):
		return http.StatusNotFound, err.Error()
	}
	if status, message, ok := authkit.StatusForAuthError(err); ok {
		return status, message
	}
	return http.StatusInternalServerError, "internal error"
}
