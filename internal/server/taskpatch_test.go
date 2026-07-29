// SPDX-License-Identifier: Elastic-2.0

package server_test

import (
	"encoding/json"
	"errors"
	"net/http"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/gopherium/alphone/internal/task"
)

func storedTask(t *testing.T, store *fakeTaskStore) task.Task {
	t.Helper()
	created, err := task.New(task.Input{
		Title:      "Call the supplier",
		DueOn:      time.Date(2026, 7, 30, 0, 0, 0, 0, time.UTC),
		Priority:   1,
		AssigneeID: uuid.Must(uuid.NewV7()),
	})
	if err != nil {
		t.Fatalf("task.New() error = %v, want nil", err)
	}
	store.tasks[created.ID] = created
	return created
}

func TestPatchTaskReplacesTheGivenFields(t *testing.T) {
	t.Parallel()

	store := newFakeTaskStore()
	stored := storedTask(t, store)
	srv, _ := authedTaskServer(t, store)

	recorder := doRequest(t, srv, http.MethodPatch, "/api/tasks/"+stored.ID.String(),
		`{"title":"  Call the supplier back  ","due_on":"2026-08-01","status":"done","priority":2}`)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusOK)
	}
	got := decodeBody[taskBody](t, recorder)
	if got.Title != "Call the supplier back" {
		t.Errorf("title = %q, want it trimmed", got.Title)
	}
	if got.DueOn != "2026-08-01" {
		t.Errorf("due_on = %q, want %q", got.DueOn, "2026-08-01")
	}
	if got.Status != task.StatusDone {
		t.Errorf("status = %q, want %q", got.Status, task.StatusDone)
	}
	if got.Priority != 2 {
		t.Errorf("priority = %d, want 2", got.Priority)
	}
	if got.ID != stored.ID || got.AssigneeID != stored.AssigneeID {
		t.Error("identity changed, want it preserved")
	}
	if persisted := store.tasks[stored.ID]; persisted.Title != got.Title ||
		persisted.Status != task.StatusDone {
		t.Errorf("persisted task = %+v, want the update stored", persisted)
	}
}

func TestPatchTaskLeavesOmittedFieldsAlone(t *testing.T) {
	t.Parallel()

	store := newFakeTaskStore()
	stored := storedTask(t, store)
	srv, _ := authedTaskServer(t, store)

	recorder := doRequest(t, srv, http.MethodPatch, "/api/tasks/"+stored.ID.String(),
		`{"status":"done"}`)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusOK)
	}
	got := decodeBody[taskBody](t, recorder)
	if got.Title != stored.Title {
		t.Errorf("title = %q, want it unchanged", got.Title)
	}
	if got.DueOn != stored.DueOn.Format("2006-01-02") {
		t.Errorf("due_on = %q, want it unchanged", got.DueOn)
	}
	if got.Priority != stored.Priority {
		t.Errorf("priority = %d, want it unchanged", got.Priority)
	}
	if got.Status != task.StatusDone {
		t.Errorf("status = %q, want %q", got.Status, task.StatusDone)
	}
}

func TestPatchTaskWithoutFieldsEchoesTheTask(t *testing.T) {
	t.Parallel()

	store := newFakeTaskStore()
	stored := storedTask(t, store)
	store.updateErr = errors.New("update must not run for an empty patch")
	srv, _ := authedTaskServer(t, store)

	recorder := doRequest(t, srv, http.MethodPatch, "/api/tasks/"+stored.ID.String(), `{}`)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusOK)
	}
	got := decodeBody[taskBody](t, recorder)
	if got.ID != stored.ID || got.Title != stored.Title || got.Status != stored.Status {
		t.Errorf("task = %+v, want the stored one echoed", got)
	}
}

func TestPatchTaskReopensADoneTask(t *testing.T) {
	t.Parallel()

	store := newFakeTaskStore()
	stored := storedTask(t, store)
	done := task.StatusDone
	reopened, err := stored.Apply(task.Changes{Status: &done})
	if err != nil {
		t.Fatalf("Apply() error = %v, want nil", err)
	}
	store.tasks[stored.ID] = reopened
	srv, _ := authedTaskServer(t, store)

	recorder := doRequest(t, srv, http.MethodPatch, "/api/tasks/"+stored.ID.String(),
		`{"status":"open"}`)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusOK)
	}
	if got := decodeBody[taskBody](t, recorder); got.Status != task.StatusOpen {
		t.Errorf("status = %q, want %q", got.Status, task.StatusOpen)
	}
}

func TestPatchTaskRejectsInvalidRequests(t *testing.T) {
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
			body:       `{"title":"   "}`,
			wantStatus: http.StatusUnprocessableEntity,
			wantError:  "task: empty title",
		},
		"malformed due date": {
			body:       `{"due_on":"01-08-2026"}`,
			wantStatus: http.StatusBadRequest,
			wantError:  "malformed due date",
		},
		"unknown status": {
			body:       `{"status":"archived"}`,
			wantStatus: http.StatusUnprocessableEntity,
			wantError:  "task: invalid status",
		},
		"list filter as status": {
			body:       `{"status":"all"}`,
			wantStatus: http.StatusUnprocessableEntity,
			wantError:  "task: invalid status",
		},
		"priority out of range": {
			body:       `{"priority":42}`,
			wantStatus: http.StatusUnprocessableEntity,
			wantError:  "task: invalid priority",
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			store := newFakeTaskStore()
			stored := storedTask(t, store)
			srv, _ := authedTaskServer(t, store)

			recorder := doRequest(t, srv, http.MethodPatch, "/api/tasks/"+stored.ID.String(), tc.body)

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

func TestPatchTaskRejectsUnknownAndMalformedIDs(t *testing.T) {
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

			recorder := doRequest(t, srv, http.MethodPatch, tc.target, `{"status":"done"}`)

			if recorder.Code != tc.wantStatus {
				t.Fatalf("status = %d, want %d", recorder.Code, tc.wantStatus)
			}
		})
	}
}

func TestPatchTaskReportsStorageFailure(t *testing.T) {
	t.Parallel()

	store := newFakeTaskStore()
	stored := storedTask(t, store)
	store.updateErr = errors.New("store down")
	srv, _ := authedTaskServer(t, store)

	recorder := doRequest(t, srv, http.MethodPatch, "/api/tasks/"+stored.ID.String(),
		`{"status":"done"}`)

	if recorder.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusInternalServerError)
	}
}

func TestPatchTaskRequiresASession(t *testing.T) {
	t.Parallel()

	store := newFakeTaskStore()
	stored := storedTask(t, store)
	users := newFakeUserStore()
	addAda(t, users)
	srv := unauthedTaskServer(users, store)

	recorder := doRequest(t, srv, http.MethodPatch, "/api/tasks/"+stored.ID.String(),
		`{"status":"done"}`)

	if recorder.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusUnauthorized)
	}
}
