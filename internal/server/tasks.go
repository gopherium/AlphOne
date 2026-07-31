// SPDX-License-Identifier: Elastic-2.0

package server

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/gopherium/gouncer/authkit"

	"github.com/gopherium/alphone/internal/event"
	"github.com/gopherium/alphone/internal/task"
)

// TaskStore provides the task persistence the HTTP API relies on.
type TaskStore interface {
	Create(ctx context.Context, t task.Task) error
	Get(ctx context.Context, id uuid.UUID) (task.Task, error)
	Update(ctx context.Context, t task.Task) (task.Task, error)
	ListForDay(
		ctx context.Context, assigneeID uuid.UUID, dueOn time.Time, status string, page task.Page,
	) ([]task.Task, error)
	ListDueBefore(
		ctx context.Context, assigneeID uuid.UUID, dueBefore time.Time, status string, page task.Page,
	) ([]task.Task, error)
	ListForContact(
		ctx context.Context, contactID uuid.UUID, status string, page task.Page,
	) ([]task.Task, error)
}

// dueDateLayout is the wire format of a task due date.
const dueDateLayout = "2006-01-02"

// statusAll lists tasks of every status.
const statusAll = "all"

// defaultTaskListLimit and maxTaskListLimit bound the tasks page size.
const (
	defaultTaskListLimit = 50
	maxTaskListLimit     = 200
)

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
			Origin:     task.Origin{Source: credentialOrigin(r.Context())},
		})
		if err != nil {
			respondDomainError(w, err)
			return
		}
		if err := s.tasks.Create(r.Context(), created); err != nil {
			respondDomainError(w, err)
			return
		}
		s.publish(r.Context(), event.TaskCreated, taskEventData(created))
		authkit.Respond(w, http.StatusCreated, newTaskResponse(created))
	}
}

type taskCursor struct {
	DueOn string    `json:"due_on"`
	ID    uuid.UUID `json:"id"`
}

type taskListResponse struct {
	Tasks      []taskResponse `json:"tasks"`
	NextCursor *string        `json:"next_cursor"`
}

// decodeTaskCursor parses the opaque list cursor, returning a zero page
// position for an absent one.
func decodeTaskCursor(raw string) (task.Page, error) {
	if raw == "" {
		return task.Page{}, nil
	}
	decoded, err := base64.RawURLEncoding.DecodeString(raw)
	if err != nil {
		return task.Page{}, fmt.Errorf("server: decode cursor: %w", err)
	}
	var cursor taskCursor
	if err := json.Unmarshal(decoded, &cursor); err != nil {
		return task.Page{}, fmt.Errorf("server: decode cursor: %w", err)
	}
	dueOn, err := time.Parse(dueDateLayout, cursor.DueOn)
	if err != nil {
		return task.Page{}, fmt.Errorf("server: decode cursor: %w", err)
	}
	return task.Page{AfterDueOn: dueOn, AfterID: cursor.ID}, nil
}

// encodeTaskCursor renders the position after t as an opaque cursor.
func encodeTaskCursor(t task.Task) string {
	encoded, _ := json.Marshal(taskCursor{DueOn: t.DueOn.Format(dueDateLayout), ID: t.ID})
	return base64.RawURLEncoding.EncodeToString(encoded)
}

// parseTaskListLimit reads the "limit" query parameter, returning the default
// when absent or an error when out of range.
func parseTaskListLimit(query url.Values) (int, error) {
	raw := query.Get("limit")
	if raw == "" {
		return defaultTaskListLimit, nil
	}
	limit, err := strconv.Atoi(raw)
	if err != nil || limit < 1 || limit > maxTaskListLimit {
		return 0, fmt.Errorf("server: invalid limit %q", raw)
	}
	return limit, nil
}

// parseTaskListStatus reads the "status" query parameter, defaulting to open
// tasks and rejecting anything but open, done, and all.
func parseTaskListStatus(query url.Values) (string, error) {
	switch status := query.Get("status"); status {
	case "":
		return task.StatusOpen, nil
	case task.StatusOpen, task.StatusDone, statusAll:
		return status, nil
	default:
		return "", fmt.Errorf("server: invalid status %q", status)
	}
}

// handleTaskList returns an HTTP handler listing tasks as a cursor paginated
// page, filtered by exactly one of date, due_before, or contact_id.
func (s *server) handleTaskList() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		query := r.URL.Query()
		status, err := parseTaskListStatus(query)
		if err != nil {
			authkit.RespondError(w, http.StatusBadRequest, "invalid status")
			return
		}
		limit, err := parseTaskListLimit(query)
		if err != nil {
			authkit.RespondError(w, http.StatusBadRequest, "invalid limit")
			return
		}
		page, err := decodeTaskCursor(query.Get("cursor"))
		if err != nil {
			authkit.RespondError(w, http.StatusBadRequest, "malformed cursor")
			return
		}
		page.Limit = limit + 1
		rows, err := s.listTasks(r, status, page)
		if err != nil {
			respondListError(w, err)
			return
		}
		var nextCursor *string
		if len(rows) > limit {
			rows = rows[:limit]
			encoded := encodeTaskCursor(rows[limit-1])
			nextCursor = &encoded
		}
		tasks := make([]taskResponse, len(rows))
		for i, listed := range rows {
			tasks[i] = newTaskResponse(listed)
		}
		authkit.Respond(w, http.StatusOK, taskListResponse{Tasks: tasks, NextCursor: nextCursor})
	}
}

// listTasks runs the store listing selected by the request's filter,
// requiring exactly one of date, due_before, and contact_id.
func (s *server) listTasks(r *http.Request, status string, page task.Page) ([]task.Task, error) {
	query := r.URL.Query()
	date, dueBefore, contactID := query.Get("date"), query.Get("due_before"), query.Get("contact_id")
	if filterCount(date, dueBefore, contactID) != 1 {
		return nil, errTaskFilter
	}
	assignee := authkit.IdentityFromContext(r.Context()).ID
	switch {
	case contactID != "":
		id, err := uuid.Parse(contactID)
		if err != nil {
			return nil, errTaskContactID
		}
		return s.tasks.ListForContact(r.Context(), id, status, page)
	case date != "":
		on, err := time.Parse(dueDateLayout, date)
		if err != nil {
			return nil, errTaskDate
		}
		return s.tasks.ListForDay(r.Context(), assignee, on, status, page)
	default:
		on, err := time.Parse(dueDateLayout, dueBefore)
		if err != nil {
			return nil, errTaskDate
		}
		return s.tasks.ListDueBefore(r.Context(), assignee, on, status, page)
	}
}

// filterCount reports how many of the listing filters are present.
func filterCount(filters ...string) int {
	count := 0
	for _, filter := range filters {
		if filter != "" {
			count++
		}
	}
	return count
}

// respondListError writes a listing failure, mapping the request errors to
// bad request and everything else to a domain error response.
func respondListError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, errTaskFilter):
		authkit.RespondError(w, http.StatusBadRequest, "one of date, due_before, or contact_id is required")
	case errors.Is(err, errTaskDate):
		authkit.RespondError(w, http.StatusBadRequest, "malformed date")
	case errors.Is(err, errTaskContactID):
		authkit.RespondError(w, http.StatusBadRequest, "malformed contact id")
	default:
		respondDomainError(w, err)
	}
}

// Listing request errors.
var (
	errTaskFilter    = errors.New("server: exactly one task filter is required")
	errTaskDate      = errors.New("server: malformed task date")
	errTaskContactID = errors.New("server: malformed task contact id")
)

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

// handleTaskPatch returns an http.HandlerFunc that applies a partial update
// to a task, treating omitted fields as unchanged.
func (s *server) handleTaskPatch() http.HandlerFunc {
	type request struct {
		Title    *string `json:"title"`
		DueOn    *string `json:"due_on"`
		Status   *string `json:"status"`
		Priority *int    `json:"priority"`
	}
	return func(w http.ResponseWriter, r *http.Request) {
		id, err := uuid.Parse(chi.URLParam(r, "id"))
		if err != nil {
			authkit.RespondError(w, http.StatusBadRequest, "malformed task id")
			return
		}
		req, err := authkit.Decode[request](w, r)
		if err != nil {
			authkit.RespondError(w, http.StatusBadRequest, "malformed json")
			return
		}
		dueOn, err := optionalDueDate(req.DueOn)
		if err != nil {
			authkit.RespondError(w, http.StatusBadRequest, "malformed due date")
			return
		}
		changes := task.Changes{
			Title:    req.Title,
			DueOn:    dueOn,
			Status:   req.Status,
			Priority: req.Priority,
		}
		stored, err := s.tasks.Get(r.Context(), id)
		if err != nil {
			respondDomainError(w, err)
			return
		}
		if changes == (task.Changes{}) {
			authkit.Respond(w, http.StatusOK, newTaskResponse(stored))
			return
		}
		changed, err := stored.Apply(changes)
		if err != nil {
			respondDomainError(w, err)
			return
		}
		updated, err := s.tasks.Update(r.Context(), changed)
		if err != nil {
			respondDomainError(w, err)
			return
		}
		if stored.Status != task.StatusDone && updated.Status == task.StatusDone {
			s.publish(r.Context(), event.TaskCompleted, taskEventData(updated))
		}
		authkit.Respond(w, http.StatusOK, newTaskResponse(updated))
	}
}

// optionalDueDate parses a due date change, reporting an omitted one as nil.
func optionalDueDate(raw *string) (*time.Time, error) {
	if raw == nil {
		return nil, nil
	}
	parsed, err := time.Parse(dueDateLayout, *raw)
	if err != nil {
		return nil, err
	}
	return &parsed, nil
}

// optionalContactID parses a contact link, reporting an absent one as
// [uuid.Nil].
func optionalContactID(raw string) (uuid.UUID, error) {
	if raw == "" {
		return uuid.Nil, nil
	}
	return uuid.Parse(raw)
}

// taskEventData returns the fields a task event carries, enough to identify
// the task and read it at a glance.
func taskEventData(t task.Task) map[string]any {
	return map[string]any{
		"id":       t.ID.String(),
		"title":    t.Title,
		"status":   t.Status,
		"due_on":   t.DueOn.Format(time.DateOnly),
		"priority": t.Priority,
	}
}
