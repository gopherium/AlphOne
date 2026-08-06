// SPDX-License-Identifier: Elastic-2.0

package server

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/gopherium/gouncer/authkit"

	"github.com/gopherium/alphone/internal/contact"
	"github.com/gopherium/alphone/internal/cursor"
	"github.com/gopherium/alphone/internal/event"
)

// ContactStore provides the contact persistence the HTTP API relies on.
type ContactStore interface {
	Create(ctx context.Context, c contact.Contact) error
	CreateContactWithIdentities(ctx context.Context, c contact.Contact, identities []contact.Identity) error
	Get(ctx context.Context, id uuid.UUID) (contact.Contact, error)
	ListContacts(
		ctx context.Context, query, digits, afterName string, afterID uuid.UUID, limit int,
	) ([]contact.Contact, error)
	ListContactIdentities(ctx context.Context, contactID uuid.UUID) ([]contact.Identity, error)
	ListByIDs(ctx context.Context, ids []uuid.UUID) ([]contact.Contact, error)
	AddIdentity(ctx context.Context, identity contact.Identity) error
	DeleteIdentity(ctx context.Context, contactID, identityID uuid.UUID) error
	RenameContact(ctx context.Context, id uuid.UUID, name string) (contact.Contact, error)
}

// errChannelNotWritable reports an identity channel the API refuses to write.
var errChannelNotWritable = errors.New("contact: channel not writable")

// writableIdentityChannels lists the channels the API accepts writes for.
var writableIdentityChannels = map[contact.Channel]bool{"email": true, "phone": true}

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

// digitsOf keeps only the decimal digits of a search query.
func digitsOf(query string) string {
	var digits strings.Builder
	for _, r := range query {
		if r >= '0' && r <= '9' {
			digits.WriteRune(r)
		}
	}
	return digits.String()
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
		afterName, afterID, err := cursor.DecodeContact(r.URL.Query().Get("cursor"))
		if err != nil {
			authkit.RespondError(w, http.StatusBadRequest, "malformed cursor")
			return
		}
		query := r.URL.Query().Get("q")
		rows, err := s.store.ListContacts(r.Context(), query, digitsOf(query), afterName, afterID, limit+1)
		if err != nil {
			respondDomainError(w, err)
			return
		}
		var nextCursor *string
		if len(rows) > limit {
			rows = rows[:limit]
			encoded := cursor.EncodeContact(rows[limit-1])
			nextCursor = &encoded
		}
		contacts := make([]contactResponse, len(rows))
		for i, c := range rows {
			contacts[i] = newContactResponse(c)
		}
		authkit.Respond(w, http.StatusOK, contactListResponse{Contacts: contacts, NextCursor: nextCursor})
	}
}

type identityRequest struct {
	Channel     string `json:"channel"`
	Identifier  string `json:"identifier"`
	DisplayName string `json:"display_name"`
}

// identityFor builds a writable identity for contactID from the request.
func identityFor(contactID uuid.UUID, req identityRequest) (contact.Identity, error) {
	identity, err := contact.NewIdentity(contactID, contact.Channel(req.Channel), req.Identifier, req.DisplayName)
	if err != nil {
		return contact.Identity{}, err
	}
	if !writableIdentityChannels[identity.Channel] {
		return contact.Identity{}, errChannelNotWritable
	}
	return identity, nil
}

// identitiesFor builds every writable identity for contactID from the requests.
func identitiesFor(contactID uuid.UUID, reqs []identityRequest) ([]contact.Identity, error) {
	identities := make([]contact.Identity, len(reqs))
	for i, req := range reqs {
		identity, err := identityFor(contactID, req)
		if err != nil {
			return nil, err
		}
		identities[i] = identity
	}
	return identities, nil
}

// storeContact persists the contact, transactionally with its identities
// when any were given.
func (s *server) storeContact(ctx context.Context, c contact.Contact, identities []contact.Identity) error {
	if len(identities) == 0 {
		return s.store.Create(ctx, c)
	}
	return s.store.CreateContactWithIdentities(ctx, c, identities)
}

// respondContactStoreError answers a contact write failure, naming the
// owning contact when an identity is already claimed.
func (s *server) respondContactStoreError(w http.ResponseWriter, r *http.Request, err error) {
	var exists contact.IdentityExistsError
	if !errors.As(err, &exists) {
		respondDomainError(w, err)
		return
	}
	owner, ownerErr := s.store.Get(r.Context(), exists.OwnerID)
	if ownerErr != nil {
		authkit.RespondError(w, http.StatusConflict, exists.Error())
		return
	}
	authkit.Respond(w, http.StatusConflict, identityConflictResponse{
		Error: exists.Error(),
		Owner: newContactResponse(owner),
	})
}

// handleContactCreate returns an http.HandlerFunc that decodes a name with
// optional identities, persists them together, and responds with the contact.
func (s *server) handleContactCreate() http.HandlerFunc {
	type request struct {
		Name       string            `json:"name"`
		Identities []identityRequest `json:"identities"`
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
		identities, err := identitiesFor(c.ID, req.Identities)
		if err != nil {
			respondDomainError(w, err)
			return
		}
		if err := s.storeContact(r.Context(), c, identities); err != nil {
			s.respondContactStoreError(w, r, err)
			return
		}
		s.publish(r.Context(), event.ContactCreated, contactEventData(c))
		authkit.Respond(w, http.StatusCreated, newContactResponse(c))
	}
}

// handleIdentityCreate returns an http.HandlerFunc attaching an identity to
// a contact.
func (s *server) handleIdentityCreate() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		contactID, err := uuid.Parse(chi.URLParam(r, "id"))
		if err != nil {
			authkit.RespondError(w, http.StatusBadRequest, "malformed contact id")
			return
		}
		req, err := authkit.Decode[identityRequest](w, r)
		if err != nil {
			authkit.RespondError(w, http.StatusBadRequest, "malformed json")
			return
		}
		identity, err := identityFor(contactID, req)
		if err != nil {
			respondDomainError(w, err)
			return
		}
		if err := s.store.AddIdentity(r.Context(), identity); err != nil {
			s.respondContactStoreError(w, r, err)
			return
		}
		authkit.Respond(w, http.StatusCreated, newIdentityResponse(identity))
	}
}

// handleIdentityDelete returns an http.HandlerFunc removing a contact's
// identity.
func (s *server) handleIdentityDelete() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		contactID, err := uuid.Parse(chi.URLParam(r, "id"))
		if err != nil {
			authkit.RespondError(w, http.StatusBadRequest, "malformed contact id")
			return
		}
		identityID, err := uuid.Parse(chi.URLParam(r, "identityID"))
		if err != nil {
			authkit.RespondError(w, http.StatusBadRequest, "malformed identity id")
			return
		}
		if err := s.store.DeleteIdentity(r.Context(), contactID, identityID); err != nil {
			respondDomainError(w, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}
}

// handleContactRename returns an http.HandlerFunc that applies a partial
// update to a contact, treating an omitted name as unchanged.
func (s *server) handleContactRename() http.HandlerFunc {
	type request struct {
		Name *string `json:"name"`
	}
	return func(w http.ResponseWriter, r *http.Request) {
		id, err := uuid.Parse(chi.URLParam(r, "id"))
		if err != nil {
			authkit.RespondError(w, http.StatusBadRequest, "malformed contact id")
			return
		}
		req, err := authkit.Decode[request](w, r)
		if err != nil {
			authkit.RespondError(w, http.StatusBadRequest, "malformed json")
			return
		}
		if req.Name == nil {
			c, err := s.store.Get(r.Context(), id)
			if err != nil {
				respondDomainError(w, err)
				return
			}
			authkit.Respond(w, http.StatusOK, newContactResponse(c))
			return
		}
		name, err := contact.Rename(*req.Name)
		if err != nil {
			respondDomainError(w, err)
			return
		}
		c, err := s.store.RenameContact(r.Context(), id, name)
		if err != nil {
			respondDomainError(w, err)
			return
		}
		authkit.Respond(w, http.StatusOK, newContactResponse(c))
	}
}

type identityResponse struct {
	ID          uuid.UUID `json:"id"`
	Channel     string    `json:"channel"`
	Identifier  string    `json:"identifier"`
	DisplayName string    `json:"display_name"`
}

// newIdentityResponse builds an identityResponse from a contact.Identity.
func newIdentityResponse(identity contact.Identity) identityResponse {
	return identityResponse{
		ID:          identity.ID,
		Channel:     string(identity.Channel),
		Identifier:  identity.Identifier,
		DisplayName: identity.DisplayName,
	}
}

type identityConflictResponse struct {
	Error string          `json:"error"`
	Owner contactResponse `json:"owner"`
}

type contactDetailResponse struct {
	contactResponse
	Identities []identityResponse `json:"identities"`
}

// handleContactGet returns an http.HandlerFunc that parses the contact id from the URL, fetches the contact,
// and responds with it and its identities.
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
		identities, err := s.store.ListContactIdentities(r.Context(), id)
		if err != nil {
			respondDomainError(w, err)
			return
		}
		detail := contactDetailResponse{
			contactResponse: newContactResponse(c),
			Identities:      make([]identityResponse, len(identities)),
		}
		for i, identity := range identities {
			detail.Identities[i] = newIdentityResponse(identity)
		}
		authkit.Respond(w, http.StatusOK, detail)
	}
}

// contactEventData returns the fields a contact event carries.
func contactEventData(c contact.Contact) map[string]any {
	return map[string]any{"id": c.ID.String(), "name": c.Name}
}
