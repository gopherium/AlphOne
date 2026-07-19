// SPDX-License-Identifier: Elastic-2.0

package server

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
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
	ListContacts(
		ctx context.Context, query, digits, afterName string, afterID uuid.UUID, limit int,
	) ([]contact.Contact, error)
}

// defaultContactListLimit and maxContactListLimit bound the contacts page
// size.
const (
	defaultContactListLimit = 50
	maxContactListLimit     = 200
)

type contactResponse struct {
	ID        uuid.UUID `json:"id"`
	Name      string    `json:"name"`
	CreatedAt time.Time `json:"created_at"`
}

// newContactResponse builds a contactResponse from a contact.Contact, normalizing the timestamp to UTC.
func newContactResponse(c contact.Contact) contactResponse {
	return contactResponse{ID: c.ID, Name: c.Name, CreatedAt: c.CreatedAt.UTC()}
}

type contactCursor struct {
	Name string    `json:"name"`
	ID   uuid.UUID `json:"id"`
}

// decodeContactCursor parses the opaque list cursor, returning zero values
// for an absent one.
func decodeContactCursor(raw string) (contactCursor, error) {
	if raw == "" {
		return contactCursor{}, nil
	}
	decoded, err := base64.RawURLEncoding.DecodeString(raw)
	if err != nil {
		return contactCursor{}, fmt.Errorf("server: decode cursor: %w", err)
	}
	var cursor contactCursor
	if err := json.Unmarshal(decoded, &cursor); err != nil {
		return contactCursor{}, fmt.Errorf("server: decode cursor: %w", err)
	}
	return cursor, nil
}

// encodeContactCursor renders the position after c as an opaque cursor.
func encodeContactCursor(c contact.Contact) string {
	encoded, _ := json.Marshal(contactCursor{Name: c.Name, ID: c.ID})
	return base64.RawURLEncoding.EncodeToString(encoded)
}

// parseContactListLimit reads the "limit" query parameter, returning the
// default when absent or an error when out of range.
func parseContactListLimit(query url.Values) (int, error) {
	raw := query.Get("limit")
	if raw == "" {
		return defaultContactListLimit, nil
	}
	limit, err := strconv.Atoi(raw)
	if err != nil || limit < 1 || limit > maxContactListLimit {
		return 0, fmt.Errorf("server: invalid limit %q", raw)
	}
	return limit, nil
}

type contactListResponse struct {
	Contacts   []contactResponse `json:"contacts"`
	NextCursor *string           `json:"next_cursor"`
}

// handleContactList returns an HTTP handler listing contacts as a cursor
// paginated page.
func (s *server) handleContactList() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		limit, err := parseContactListLimit(r.URL.Query())
		if err != nil {
			authkit.RespondError(w, http.StatusBadRequest, "invalid limit")
			return
		}
		cursor, err := decodeContactCursor(r.URL.Query().Get("cursor"))
		if err != nil {
			authkit.RespondError(w, http.StatusBadRequest, "malformed cursor")
			return
		}
		rows, err := s.store.ListContacts(r.Context(), "", "", cursor.Name, cursor.ID, limit+1)
		if err != nil {
			respondDomainError(w, err)
			return
		}
		var nextCursor *string
		if len(rows) > limit {
			rows = rows[:limit]
			encoded := encodeContactCursor(rows[limit-1])
			nextCursor = &encoded
		}
		contacts := make([]contactResponse, len(rows))
		for i, c := range rows {
			contacts[i] = newContactResponse(c)
		}
		authkit.Respond(w, http.StatusOK, contactListResponse{Contacts: contacts, NextCursor: nextCursor})
	}
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
