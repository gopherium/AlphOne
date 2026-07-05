// SPDX-License-Identifier: AGPL-3.0-or-later

package server

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"

	"github.com/gopherium/alphone/internal/contact"
)

type errorResponse struct {
	Error string `json:"error"`
}

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

func respondError(w http.ResponseWriter, status int, message string) {
	respond(w, status, errorResponse{Error: message})
}

func respondDomainError(w http.ResponseWriter, err error) {
	status := statusFor(err)
	message := err.Error()
	if status == http.StatusInternalServerError {
		message = "internal error"
	}
	respondError(w, status, message)
}

func statusFor(err error) int {
	switch {
	case errors.Is(err, contact.ErrEmptyName):
		return http.StatusUnprocessableEntity
	case errors.Is(err, contact.ErrNotFound):
		return http.StatusNotFound
	default:
		return http.StatusInternalServerError
	}
}

func decode[T any](r *http.Request) (T, error) {
	var v T
	if err := json.NewDecoder(r.Body).Decode(&v); err != nil {
		return v, fmt.Errorf("decode json: %w", err)
	}
	return v, nil
}
