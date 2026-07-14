// SPDX-License-Identifier: Elastic-2.0

package server

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"

	"github.com/gopherium/gouncer"

	"github.com/gopherium/alphone/internal/contact"
)

type errorResponse struct {
	Error string `json:"error"`
}

// respond writes v as a JSON response with the given status code, falling back to a 500 error payload
// if marshaling fails.
func respond(w http.ResponseWriter, status int, v any) {
	data, err := json.Marshal(v)
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`{"error":"internal error"}`))
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_, _ = w.Write(data)
}

// respondError writes a JSON error response with the given status code and message.
func respondError(w http.ResponseWriter, status int, message string) {
	respond(w, status, errorResponse{Error: message})
}

// respondDomainError maps a domain error to an HTTP status and writes it as a JSON error response,
// masking internal errors.
func respondDomainError(w http.ResponseWriter, err error) {
	status, message := statusFor(err)
	respondError(w, status, message)
}

// statusFor returns the HTTP status code and client-facing message for the
// given domain error, keeping third-party library wording out of the API
// contract and masking unrecognized errors as internal ones.
func statusFor(err error) (int, string) {
	switch {
	case errors.Is(err, contact.ErrEmptyName):
		return http.StatusUnprocessableEntity, err.Error()
	case errors.Is(err, gouncer.ErrInvalidEmail):
		return http.StatusUnprocessableEntity, "invalid email address"
	case errors.Is(err, gouncer.ErrEmptyName):
		return http.StatusUnprocessableEntity, "name is required"
	case errors.Is(err, gouncer.ErrNameTooLong):
		return http.StatusUnprocessableEntity, "name must be at most 256 characters"
	case errors.Is(err, gouncer.ErrWeakPassword):
		return http.StatusUnprocessableEntity, "password must be at least 12 characters"
	case errors.Is(err, gouncer.ErrPasswordTooLong):
		return http.StatusUnprocessableEntity, "password must be at most 1024 characters"
	case errors.Is(err, contact.ErrNotFound):
		return http.StatusNotFound, err.Error()
	case errors.Is(err, gouncer.ErrUserNotFound):
		return http.StatusNotFound, "user not found"
	case errors.Is(err, gouncer.ErrEmailTaken):
		return http.StatusConflict, "email already in use"
	default:
		return http.StatusInternalServerError, "internal error"
	}
}

// maxRequestBodyBytes caps how much of a request body the JSON decoder
// will read, so an unauthenticated caller cannot exhaust memory.
const maxRequestBodyBytes = 1 << 20

// decode reads and JSON-decodes a single request body into a value of
// type T, bounding the body size and rejecting trailing content.
func decode[T any](w http.ResponseWriter, r *http.Request) (T, error) {
	var v T
	r.Body = http.MaxBytesReader(w, r.Body, maxRequestBodyBytes)
	dec := json.NewDecoder(r.Body)
	if err := dec.Decode(&v); err != nil {
		return v, fmt.Errorf("decode json: %w", err)
	}
	if dec.More() {
		return v, errors.New("decode json: unexpected trailing content")
	}
	return v, nil
}
