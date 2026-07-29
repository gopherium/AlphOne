// SPDX-License-Identifier: Elastic-2.0

package server_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/gopherium/alphone/internal/postgres"
	"github.com/gopherium/alphone/internal/server"
	"github.com/gopherium/alphone/internal/task"
)

var (
	_ server.TaskStore = (*postgres.TaskStore)(nil)
	_ server.TaskStore = (*fakeTaskStore)(nil)
)

type taskListCall struct {
	mode       string
	assigneeID uuid.UUID
	contactID  uuid.UUID
	on         time.Time
	status     string
	page       task.Page
}

type fakeTaskStore struct {
	tasks     map[uuid.UUID]task.Task
	listed    []task.Task
	lastList  taskListCall
	createErr error
	getErr    error
	listErr   error
}

func newFakeTaskStore() *fakeTaskStore {
	return &fakeTaskStore{tasks: map[uuid.UUID]task.Task{}}
}

func (f *fakeTaskStore) ListForDay(
	_ context.Context, assigneeID uuid.UUID, dueOn time.Time, status string, page task.Page,
) ([]task.Task, error) {
	f.lastList = taskListCall{
		mode: "day", assigneeID: assigneeID, on: dueOn, status: status, page: page,
	}
	return f.listPage(page)
}

func (f *fakeTaskStore) ListDueBefore(
	_ context.Context, assigneeID uuid.UUID, dueBefore time.Time, status string, page task.Page,
) ([]task.Task, error) {
	f.lastList = taskListCall{
		mode: "before", assigneeID: assigneeID, on: dueBefore, status: status, page: page,
	}
	return f.listPage(page)
}

func (f *fakeTaskStore) ListForContact(
	_ context.Context, contactID uuid.UUID, status string, page task.Page,
) ([]task.Task, error) {
	f.lastList = taskListCall{mode: "contact", contactID: contactID, status: status, page: page}
	return f.listPage(page)
}

func (f *fakeTaskStore) listPage(page task.Page) ([]task.Task, error) {
	if f.listErr != nil {
		return nil, f.listErr
	}
	if len(f.listed) > page.Limit {
		return f.listed[:page.Limit], nil
	}
	return f.listed, nil
}

func (f *fakeTaskStore) Create(_ context.Context, t task.Task) error {
	if f.createErr != nil {
		return f.createErr
	}
	f.tasks[t.ID] = t
	return nil
}

func (f *fakeTaskStore) Get(_ context.Context, id uuid.UUID) (task.Task, error) {
	if f.getErr != nil {
		return task.Task{}, f.getErr
	}
	stored, ok := f.tasks[id]
	if !ok {
		return task.Task{}, task.ErrNotFound
	}
	return stored, nil
}

type taskBody struct {
	ID            uuid.UUID  `json:"id"`
	Title         string     `json:"title"`
	Status        string     `json:"status"`
	Priority      int        `json:"priority"`
	DueOn         string     `json:"due_on"`
	AssigneeID    uuid.UUID  `json:"assignee_id"`
	ContactID     *uuid.UUID `json:"contact_id"`
	OriginSource  *string    `json:"origin_source"`
	OriginEventID *uuid.UUID `json:"origin_event_id"`
	CreatedAt     time.Time  `json:"created_at"`
}

func authedTaskServer(t *testing.T, store server.TaskStore) (http.Handler, uuid.UUID) {
	t.Helper()
	users := newFakeUserStore()
	ada := addAda(t, users)
	srv := server.NewServer(server.Config{Contacts: newFakeContactStore(), Tasks: store, Users: users})
	cookie := loginCookie(t, srv)
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		r.AddCookie(cookie)
		srv.ServeHTTP(w, r)
	})
	return handler, ada.ID
}

func TestCreateTask(t *testing.T) {
	t.Parallel()

	store := newFakeTaskStore()
	srv, ada := authedTaskServer(t, store)

	recorder := doRequest(t, srv, http.MethodPost, "/api/tasks",
		`{"title":"  Call María about the wicker lamp  ","due_on":"2026-07-30","priority":1}`)

	if recorder.Code != http.StatusCreated {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusCreated)
	}
	got := decodeBody[taskBody](t, recorder)
	if got.Title != "Call María about the wicker lamp" {
		t.Errorf("title = %q, want it trimmed", got.Title)
	}
	if got.Status != task.StatusOpen {
		t.Errorf("status = %q, want %q", got.Status, task.StatusOpen)
	}
	if got.DueOn != "2026-07-30" {
		t.Errorf("due_on = %q, want %q", got.DueOn, "2026-07-30")
	}
	if got.Priority != 1 {
		t.Errorf("priority = %d, want 1", got.Priority)
	}
	if got.AssigneeID != ada {
		t.Errorf("assignee_id = %v, want the session user %v", got.AssigneeID, ada)
	}
	stored, ok := store.tasks[got.ID]
	if !ok || stored.Title != got.Title {
		t.Errorf("stored task = %+v, want the created one", stored)
	}
	if stored.AssigneeID != got.AssigneeID {
		t.Errorf("stored assignee = %v, want %v", stored.AssigneeID, got.AssigneeID)
	}
}

func TestCreateTaskAssignsTheSessionUser(t *testing.T) {
	t.Parallel()

	store := newFakeTaskStore()
	srv, ada := authedTaskServer(t, store)

	recorder := doRequest(t, srv, http.MethodPost, "/api/tasks",
		`{"title":"Call the supplier","due_on":"2026-07-30","assignee_id":"0198c000-0000-7000-8000-000000000001"}`)

	if recorder.Code != http.StatusCreated {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusCreated)
	}
	got := decodeBody[taskBody](t, recorder)
	if got.AssigneeID != ada {
		t.Errorf("assignee_id = %v, want the session user %v ignoring the request body", got.AssigneeID, ada)
	}
}

func TestCreateTaskLinksAContact(t *testing.T) {
	t.Parallel()

	store := newFakeTaskStore()
	srv, _ := authedTaskServer(t, store)
	contactID := uuid.Must(uuid.NewV7())

	recorder := doRequest(t, srv, http.MethodPost, "/api/tasks",
		`{"title":"Call María","due_on":"2026-07-30","contact_id":"`+contactID.String()+`"}`)

	if recorder.Code != http.StatusCreated {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusCreated)
	}
	got := decodeBody[taskBody](t, recorder)
	if got.ContactID == nil || *got.ContactID != contactID {
		t.Errorf("contact_id = %v, want %v", got.ContactID, contactID)
	}
}

func TestCreateTaskRejectsInvalidBodies(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		body       string
		wantStatus int
		wantError  string
	}{
		"malformed json": {
			body:       `{"title":`,
			wantStatus: http.StatusBadRequest,
			wantError:  "malformed json",
		},
		"blank title": {
			body:       `{"title":"  ","due_on":"2026-07-30"}`,
			wantStatus: http.StatusUnprocessableEntity,
			wantError:  "task: empty title",
		},
		"missing due date": {
			body:       `{"title":"Call the supplier"}`,
			wantStatus: http.StatusBadRequest,
			wantError:  "malformed due date",
		},
		"malformed due date": {
			body:       `{"title":"Call the supplier","due_on":"30-07-2026"}`,
			wantStatus: http.StatusBadRequest,
			wantError:  "malformed due date",
		},
		"malformed contact id": {
			body:       `{"title":"Call the supplier","due_on":"2026-07-30","contact_id":"nope"}`,
			wantStatus: http.StatusBadRequest,
			wantError:  "malformed contact id",
		},
		"priority out of range": {
			body:       `{"title":"Call the supplier","due_on":"2026-07-30","priority":42}`,
			wantStatus: http.StatusUnprocessableEntity,
			wantError:  "task: invalid priority",
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			srv, _ := authedTaskServer(t, newFakeTaskStore())

			recorder := doRequest(t, srv, http.MethodPost, "/api/tasks", tc.body)

			if recorder.Code != tc.wantStatus {
				t.Fatalf("status = %d, want %d", recorder.Code, tc.wantStatus)
			}
			var body struct {
				Error string `json:"error"`
			}
			if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
				t.Fatalf("decoding response: %v", err)
			}
			if body.Error != tc.wantError {
				t.Errorf("error = %q, want %q", body.Error, tc.wantError)
			}
		})
	}
}

func TestCreateTaskReportsStorageFailure(t *testing.T) {
	t.Parallel()

	store := newFakeTaskStore()
	store.createErr = errors.New("store down")
	srv, _ := authedTaskServer(t, store)

	recorder := doRequest(t, srv, http.MethodPost, "/api/tasks",
		`{"title":"Call the supplier","due_on":"2026-07-30"}`)

	if recorder.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusInternalServerError)
	}
}

func TestGetTask(t *testing.T) {
	t.Parallel()

	store := newFakeTaskStore()
	stored, err := task.New(task.Input{
		Title:      "Call María about the wicker lamp",
		DueOn:      time.Date(2026, 7, 30, 0, 0, 0, 0, time.UTC),
		AssigneeID: uuid.Must(uuid.NewV7()),
	})
	if err != nil {
		t.Fatalf("task.New() error = %v, want nil", err)
	}
	store.tasks[stored.ID] = stored
	srv, _ := authedTaskServer(t, store)

	recorder := doRequest(t, srv, http.MethodGet, "/api/tasks/"+stored.ID.String(), "")

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusOK)
	}
	got := decodeBody[taskBody](t, recorder)
	if got.ID != stored.ID || got.Title != stored.Title {
		t.Errorf("task = %+v, want the stored one", got)
	}
}

func TestGetTaskRendersItsOrigin(t *testing.T) {
	t.Parallel()

	store := newFakeTaskStore()
	origin := task.Origin{Source: "seed", EventID: uuid.Must(uuid.NewV7())}
	stored, err := task.New(task.Input{
		Title:      "Review the imported numbers",
		DueOn:      time.Date(2026, 7, 30, 0, 0, 0, 0, time.UTC),
		AssigneeID: uuid.Must(uuid.NewV7()),
		ContactID:  uuid.Must(uuid.NewV7()),
		Origin:     origin,
	})
	if err != nil {
		t.Fatalf("task.New() error = %v, want nil", err)
	}
	store.tasks[stored.ID] = stored
	srv, _ := authedTaskServer(t, store)

	recorder := doRequest(t, srv, http.MethodGet, "/api/tasks/"+stored.ID.String(), "")

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusOK)
	}
	got := decodeBody[taskBody](t, recorder)
	if got.OriginSource == nil || *got.OriginSource != origin.Source {
		t.Errorf("origin_source = %v, want %q", got.OriginSource, origin.Source)
	}
	if got.OriginEventID == nil || *got.OriginEventID != origin.EventID {
		t.Errorf("origin_event_id = %v, want %v", got.OriginEventID, origin.EventID)
	}
	if got.ContactID == nil || *got.ContactID != stored.ContactID {
		t.Errorf("contact_id = %v, want %v", got.ContactID, stored.ContactID)
	}
}

func TestGetTaskRejectsUnknownAndMalformedIDs(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		target     string
		wantStatus int
	}{
		"malformed id": {target: "/api/tasks/nope", wantStatus: http.StatusBadRequest},
		"unknown id": {
			target:     "/api/tasks/" + uuid.Must(uuid.NewV7()).String(),
			wantStatus: http.StatusNotFound,
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			srv, _ := authedTaskServer(t, newFakeTaskStore())

			recorder := doRequest(t, srv, http.MethodGet, tc.target, "")

			if recorder.Code != tc.wantStatus {
				t.Fatalf("status = %d, want %d", recorder.Code, tc.wantStatus)
			}
		})
	}
}

func TestTaskEnvelopeKeepsItsShape(t *testing.T) {
	t.Parallel()

	store := newFakeTaskStore()
	srv, _ := authedTaskServer(t, store)

	recorder := doRequest(t, srv, http.MethodPost, "/api/tasks",
		`{"title":"Call the supplier","due_on":"2026-07-30"}`)

	if recorder.Code != http.StatusCreated {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusCreated)
	}
	var envelope map[string]json.RawMessage
	if err := json.Unmarshal(recorder.Body.Bytes(), &envelope); err != nil {
		t.Fatalf("decoding response: %v", err)
	}
	want := []string{
		"id", "assignee_id", "contact_id", "title", "status", "priority",
		"due_on", "origin_source", "origin_event_id", "created_at",
	}
	if len(envelope) != len(want) {
		t.Errorf("envelope has %d keys, want %d: %v", len(envelope), len(want), envelope)
	}
	for _, key := range want {
		if _, ok := envelope[key]; !ok {
			t.Errorf("envelope is missing %q", key)
		}
	}
	for _, key := range []string{"contact_id", "origin_source", "origin_event_id"} {
		if string(envelope[key]) != "null" {
			t.Errorf("%s = %s, want null", key, envelope[key])
		}
	}
}

func TestTaskRoutesRequireASession(t *testing.T) {
	t.Parallel()

	users := newFakeUserStore()
	addAda(t, users)
	srv := server.NewServer(server.Config{
		Contacts: newFakeContactStore(),
		Tasks:    newFakeTaskStore(),
		Users:    users,
	})

	requests := map[string]struct {
		method string
		target string
		body   string
	}{
		"create": {method: http.MethodPost, target: "/api/tasks", body: `{"title":"x","due_on":"2026-07-30"}`},
		"get":    {method: http.MethodGet, target: "/api/tasks/" + uuid.Must(uuid.NewV7()).String()},
	}

	for name, tc := range requests {
		recorder := doRequest(t, srv, tc.method, tc.target, tc.body)

		if recorder.Code != http.StatusUnauthorized {
			t.Errorf("%s status = %d, want %d", name, recorder.Code, http.StatusUnauthorized)
		}
	}
}
