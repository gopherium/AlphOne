// SPDX-License-Identifier: Elastic-2.0

package server

import (
	"context"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/gopherium/gouncer/authkit"

	"github.com/gopherium/alphone/internal/contact"
)

// ContactStore provides the contact persistence the HTTP API relies on.
type ContactStore interface {
	Create(ctx context.Context, c contact.Contact) error
	Get(ctx context.Context, id uuid.UUID) (contact.Contact, error)
}

type contactResponse struct {
	ID        uuid.UUID `json:"id"`
	Name      string    `json:"name"`
	CreatedAt time.Time `json:"created_at"`
}

// newContactResponse builds a contactResponse from a contact.Contact, normalizing the timestamp to UTC.
func newContactResponse(c contact.Contact) contactResponse {
	return contactResponse{ID: c.ID, Name: c.Name, CreatedAt: c.CreatedAt.UTC()}
}

// handleContactCreate returns an http.HandlerFunc that decodes a name, creates a contact, persists it, and
// responds with the created contact.
func (s *server) handleContactCreate() http.HandlerFunc {
	type request struct {
		Name string `json:"name"`
	}
	return func(w http.ResponseWriter, r *http.Request) {
		req, err := authkit.Decode[request](w, r)
		if err != nil {
			authkit.RespondError(w, http.StatusBadRequest, "malformed json")
			return
		}
		c, err := contact.New(req.Name)
		if err != nil {
			respondDomainError(w, err)
			return
		}
		if err := s.store.Create(r.Context(), c); err != nil {
			respondDomainError(w, err)
			return
		}
		authkit.Respond(w, http.StatusCreated, newContactResponse(c))
	}
}

// handleContactGet returns an http.HandlerFunc that parses the contact id from the URL, fetches the contact,
// and responds with it.
func (s *server) handleContactGet() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, err := uuid.Parse(chi.URLParam(r, "id"))
		if err != nil {
			authkit.RespondError(w, http.StatusBadRequest, "malformed contact id")
			return
		}
		c, err := s.store.Get(r.Context(), id)
		if err != nil {
			respondDomainError(w, err)
			return
		}
		authkit.Respond(w, http.StatusOK, newContactResponse(c))
	}
}
