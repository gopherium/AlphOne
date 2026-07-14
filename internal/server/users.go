// SPDX-License-Identifier: Elastic-2.0

package server

import (
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/gopherium/gouncer"
)

type userSummary struct {
	ID        uuid.UUID `json:"id"`
	Email     string    `json:"email"`
	Name      string    `json:"name"`
	Disabled  bool      `json:"disabled"`
	CreatedAt time.Time `json:"created_at"`
}

// newUserSummary builds a userSummary from a user, normalizing the timestamp to UTC.
func newUserSummary(u gouncer.User) userSummary {
	return userSummary{
		ID:        u.ID,
		Email:     u.Email,
		Name:      u.Name,
		Disabled:  u.Disabled,
		CreatedAt: u.CreatedAt.UTC(),
	}
}

// handleUserList returns an http.HandlerFunc that responds with every user account.
func (s *server) handleUserList() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		users, err := s.users.ListUsers(r.Context())
		if err != nil {
			respondDomainError(w, err)
			return
		}
		summaries := make([]userSummary, len(users))
		for i, u := range users {
			summaries[i] = newUserSummary(u)
		}
		respond(w, http.StatusOK, summaries)
	}
}

// handleUserCreate returns an http.HandlerFunc that decodes credentials, creates a
// user account, persists it, and responds with the created account.
func (s *server) handleUserCreate() http.HandlerFunc {
	type request struct {
		Email    string `json:"email"`
		Name     string `json:"name"`
		Password string `json:"password"`
	}
	return func(w http.ResponseWriter, r *http.Request) {
		req, err := decode[request](w, r)
		if err != nil {
			respondError(w, http.StatusBadRequest, "malformed json")
			return
		}
		u, err := gouncer.NewUser(req.Email, req.Name, req.Password)
		if err != nil {
			respondDomainError(w, err)
			return
		}
		if err := s.users.CreateUser(r.Context(), u); err != nil {
			respondDomainError(w, err)
			return
		}
		respond(w, http.StatusCreated, newUserSummary(u))
	}
}

// handleUserSetDisabled returns an http.HandlerFunc that parses the user id from the
// URL and updates whether that account may log in, refusing to disable the requester.
func (s *server) handleUserSetDisabled() http.HandlerFunc {
	type request struct {
		Disabled *bool `json:"disabled"`
	}
	return func(w http.ResponseWriter, r *http.Request) {
		id, err := uuid.Parse(chi.URLParam(r, "id"))
		if err != nil {
			respondError(w, http.StatusBadRequest, "malformed user id")
			return
		}
		req, err := decode[request](w, r)
		if err != nil {
			respondError(w, http.StatusBadRequest, "malformed json")
			return
		}
		if req.Disabled == nil {
			respondError(w, http.StatusUnprocessableEntity, "disabled is required")
			return
		}
		if *req.Disabled && userFromContext(r.Context()).ID == id {
			respondError(w, http.StatusUnprocessableEntity, "cannot disable your own account")
			return
		}
		if err := s.users.SetUserDisabled(r.Context(), id, *req.Disabled); err != nil {
			respondDomainError(w, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}
}
