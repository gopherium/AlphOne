// SPDX-License-Identifier: Elastic-2.0

package server

import (
	"context"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/gopherium/gouncer/authkit"

	"github.com/gopherium/alphone/internal/task"
)

// TaskStore provides the task persistence the HTTP API relies on.
type TaskStore interface {
	Create(ctx context.Context, t task.Task) error
	Get(ctx context.Context, id uuid.UUID) (task.Task, error)
}

// dueDateLayout is the wire format of a task due date.
const dueDateLayout = "2006-01-02"

type taskResponse struct {
	ID            uuid.UUID  `json:"id"`
	AssigneeID    uuid.UUID  `json:"assignee_id"`
	ContactID     *uuid.UUID `json:"contact_id"`
	Title         string     `json:"title"`
	Status        string     `json:"status"`
	Priority      int        `json:"priority"`
	DueOn         string     `json:"due_on"`
	OriginSource  *string    `json:"origin_source"`
	OriginEventID *uuid.UUID `json:"origin_event_id"`
	CreatedAt     time.Time  `json:"created_at"`
}

// newTaskResponse builds a taskResponse from a task.Task, rendering absent
// links as null and normalizing the timestamp to UTC.
func newTaskResponse(t task.Task) taskResponse {
	response := taskResponse{
		ID:         t.ID,
		AssigneeID: t.AssigneeID,
		Title:      t.Title,
		Status:     t.Status,
		Priority:   t.Priority,
		DueOn:      t.DueOn.Format(dueDateLayout),
		CreatedAt:  t.CreatedAt.UTC(),
	}
	if t.ContactID != uuid.Nil {
		response.ContactID = &t.ContactID
	}
	if t.Origin.Source != "" {
		response.OriginSource = &t.Origin.Source
	}
	if t.Origin.EventID != uuid.Nil {
		response.OriginEventID = &t.Origin.EventID
	}
	return response
}

// handleTaskCreate returns an http.HandlerFunc that creates a task owned by
// the session user and responds with the created task.
func (s *server) handleTaskCreate() http.HandlerFunc {
	type request struct {
		Title     string `json:"title"`
		DueOn     string `json:"due_on"`
		Priority  int    `json:"priority"`
		ContactID string `json:"contact_id"`
	}
	return func(w http.ResponseWriter, r *http.Request) {
		req, err := authkit.Decode[request](w, r)
		if err != nil {
			authkit.RespondError(w, http.StatusBadRequest, "malformed json")
			return
		}
		dueOn, err := time.Parse(dueDateLayout, req.DueOn)
		if err != nil {
			authkit.RespondError(w, http.StatusBadRequest, "malformed due date")
			return
		}
		contactID, err := optionalContactID(req.ContactID)
		if err != nil {
			authkit.RespondError(w, http.StatusBadRequest, "malformed contact id")
			return
		}
		created, err := task.New(task.Input{
			Title:      req.Title,
			DueOn:      dueOn,
			Priority:   req.Priority,
			AssigneeID: authkit.IdentityFromContext(r.Context()).ID,
			ContactID:  contactID,
		})
		if err != nil {
			respondDomainError(w, err)
			return
		}
		if err := s.tasks.Create(r.Context(), created); err != nil {
			respondDomainError(w, err)
			return
		}
		authkit.Respond(w, http.StatusCreated, newTaskResponse(created))
	}
}

// handleTaskGet returns an http.HandlerFunc that responds with one task.
func (s *server) handleTaskGet() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, err := uuid.Parse(chi.URLParam(r, "id"))
		if err != nil {
			authkit.RespondError(w, http.StatusBadRequest, "malformed task id")
			return
		}
		stored, err := s.tasks.Get(r.Context(), id)
		if err != nil {
			respondDomainError(w, err)
			return
		}
		authkit.Respond(w, http.StatusOK, newTaskResponse(stored))
	}
}

// optionalContactID parses a contact link, reporting an absent one as
// [uuid.Nil].
func optionalContactID(raw string) (uuid.UUID, error) {
	if raw == "" {
		return uuid.Nil, nil
	}
	return uuid.Parse(raw)
}
