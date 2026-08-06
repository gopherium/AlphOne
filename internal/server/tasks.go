// SPDX-License-Identifier: Elastic-2.0

package server

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/gopherium/gouncer/authkit"

	"github.com/gopherium/alphone/internal/cursor"
	"github.com/gopherium/alphone/internal/event"
	"github.com/gopherium/alphone/internal/task"
)

// TaskStore provides the task persistence the HTTP API relies on.
type TaskStore interface {
	Create(ctx context.Context, t task.Task) (task.Task, bool, error)
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
	return func(w http.ResponseWriter, r *http.Request) {
		req, err := authkit.Decode[taskRequest](w, r)
		if err != nil {
			authkit.RespondError(w, http.StatusBadRequest, "malformed json")
			return
		}
		input, message := taskInputFrom(req, r)
		if message != "" {
			authkit.RespondError(w, http.StatusBadRequest, message)
			return
		}
		created, err := task.New(input)
		if err != nil {
			respondDomainError(w, err)
			return
		}
		stored, isNew, err := s.tasks.Create(r.Context(), created)
		if err != nil {
			respondDomainError(w, err)
			return
		}
		s.respondTaskCreation(w, r, stored, isNew)
	}
}

// respondTaskCreation answers a stored task, publishing task.created when new.
func (s *server) respondTaskCreation(
	w http.ResponseWriter, r *http.Request, stored task.Task, isNew bool,
) {
	if !isNew {
		authkit.Respond(w, http.StatusOK, newTaskResponse(stored))
		return
	}
	s.publish(r.Context(), event.TaskCreated, taskEventData(stored))
	authkit.Respond(w, http.StatusCreated, newTaskResponse(stored))
}

// taskRequest is the body a task creation accepts.
type taskRequest struct {
	Title         string `json:"title"`
	DueOn         string `json:"due_on"`
	Priority      int    `json:"priority"`
	ContactID     string `json:"contact_id"`
	OriginEventID string `json:"origin_event_id"`
}

// taskInputFrom builds the task input a request stands for, returning the
// message to answer a malformed field with.
func taskInputFrom(req taskRequest, r *http.Request) (task.Input, string) {
	dueOn, err := time.Parse(dueDateLayout, req.DueOn)
	if err != nil {
		return task.Input{}, "malformed due date"
	}
	contactID, err := optionalID(req.ContactID)
	if err != nil {
		return task.Input{}, "malformed contact id"
	}
	eventID, err := optionalID(req.OriginEventID)
	if err != nil {
		return task.Input{}, "malformed origin event id"
	}
	return task.Input{
		Title:      req.Title,
		DueOn:      dueOn,
		Priority:   req.Priority,
		AssigneeID: authkit.IdentityFromContext(r.Context()).ID,
		ContactID:  contactID,
		Origin:     task.Origin{Source: credentialOrigin(r.Context()), EventID: eventID},
	}, ""
}

type taskListResponse struct {
	Tasks      []taskResponse `json:"tasks"`
	NextCursor *string        `json:"next_cursor"`
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
		page, err := cursor.DecodeTask(query.Get("cursor"))
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
			encoded := cursor.EncodeTask(rows[limit-1])
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
		updated, err := s.patchTask(r.Context(), stored, changes)
		if err != nil {
			respondDomainError(w, err)
			return
		}
		authkit.Respond(w, http.StatusOK, newTaskResponse(updated))
	}
}

// patchTask applies changes to stored, publishing a completion event when the
// update closes the task.
func (s *server) patchTask(
	ctx context.Context,
	stored task.Task,
	changes task.Changes,
) (task.Task, error) {
	if changes == (task.Changes{}) {
		return stored, nil
	}
	changed, err := stored.Apply(changes)
	if err != nil {
		return task.Task{}, err
	}
	updated, err := s.tasks.Update(ctx, changed)
	if err != nil {
		return task.Task{}, err
	}
	if stored.Status != task.StatusDone && updated.Status == task.StatusDone {
		s.publish(ctx, event.TaskCompleted, taskEventData(updated))
	}
	return updated, nil
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

// optionalID parses an id a request may omit, reporting an absent one as
// [uuid.Nil].
func optionalID(raw string) (uuid.UUID, error) {
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
