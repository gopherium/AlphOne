// SPDX-License-Identifier: Elastic-2.0

package server_test

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/gopherium/alphone/internal/task"
)

type taskListBody struct {
	Tasks      []taskBody `json:"tasks"`
	NextCursor *string    `json:"next_cursor"`
}

func listedTask(t *testing.T, title string, dueOn time.Time) task.Task {
	t.Helper()
	created, err := task.New(task.Input{
		Title:      title,
		DueOn:      dueOn,
		AssigneeID: uuid.Must(uuid.NewV7()),
	})
	if err != nil {
		t.Fatalf("task.New() error = %v, want nil", err)
	}
	return created
}

func TestListTasksForADay(t *testing.T) {
	t.Parallel()

	store := newFakeTaskStore()
	store.listed = []task.Task{
		listedTask(t, "Call the supplier", time.Date(2026, 7, 30, 0, 0, 0, 0, time.UTC)),
	}
	srv, ada := authedTaskServer(t, store)

	recorder := doRequest(t, srv, http.MethodGet, "/api/tasks?date=2026-07-30", "")

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusOK)
	}
	got := decodeBody[taskListBody](t, recorder)
	if len(got.Tasks) != 1 || got.Tasks[0].Title != "Call the supplier" {
		t.Errorf("tasks = %+v, want the listed one", got.Tasks)
	}
	if got.NextCursor != nil {
		t.Errorf("next_cursor = %v, want null", *got.NextCursor)
	}
	if store.lastList.mode != "day" {
		t.Errorf("mode = %q, want %q", store.lastList.mode, "day")
	}
	if got := store.lastList.on.Format("2006-01-02"); got != "2026-07-30" {
		t.Errorf("date = %q, want 2026-07-30", got)
	}
	if store.lastList.status != task.StatusOpen {
		t.Errorf("status = %q, want %q by default", store.lastList.status, task.StatusOpen)
	}
	if store.lastList.assigneeID != ada {
		t.Errorf("assignee = %v, want the session user %v", store.lastList.assigneeID, ada)
	}
}

func TestListTasksScopesDayAndOverdueToTheSessionUser(t *testing.T) {
	t.Parallel()

	store := newFakeTaskStore()
	srv, ada := authedTaskServer(t, store)

	recorder := doRequest(t, srv, http.MethodGet, "/api/tasks?date=2026-07-30", "")
	if recorder.Code != http.StatusOK {
		t.Fatalf("day status = %d, want %d", recorder.Code, http.StatusOK)
	}
	if store.lastList.assigneeID != ada {
		t.Errorf("day assignee = %v, want the session user %v", store.lastList.assigneeID, ada)
	}

	recorder = doRequest(t, srv, http.MethodGet, "/api/tasks?due_before=2026-07-30", "")

	if recorder.Code != http.StatusOK {
		t.Fatalf("overdue status = %d, want %d", recorder.Code, http.StatusOK)
	}
	if store.lastList.mode != "before" {
		t.Errorf("mode = %q, want %q", store.lastList.mode, "before")
	}
	if store.lastList.assigneeID != ada {
		t.Errorf("overdue assignee = %v, want the session user %v", store.lastList.assigneeID, ada)
	}
	if got := store.lastList.on.Format("2006-01-02"); got != "2026-07-30" {
		t.Errorf("due_before = %q, want 2026-07-30", got)
	}
}

func TestListTasksForAContactSpansAssignees(t *testing.T) {
	t.Parallel()

	store := newFakeTaskStore()
	srv, _ := authedTaskServer(t, store)
	contactID := uuid.Must(uuid.NewV7())

	recorder := doRequest(t, srv, http.MethodGet, "/api/tasks?contact_id="+contactID.String(), "")

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusOK)
	}
	if store.lastList.mode != "contact" {
		t.Errorf("mode = %q, want %q", store.lastList.mode, "contact")
	}
	if store.lastList.contactID != contactID {
		t.Errorf("contact = %v, want %v", store.lastList.contactID, contactID)
	}
	if store.lastList.assigneeID != uuid.Nil {
		t.Errorf("assignee = %v, want no assignee filter", store.lastList.assigneeID)
	}
}

func TestListTasksForwardsTheStatusFilter(t *testing.T) {
	t.Parallel()

	tests := map[string]string{
		"":     task.StatusOpen,
		"open": task.StatusOpen,
		"done": task.StatusDone,
		"all":  "all",
	}

	for query, want := range tests {
		t.Run("status="+query, func(t *testing.T) {
			t.Parallel()

			store := newFakeTaskStore()
			srv, _ := authedTaskServer(t, store)
			target := "/api/tasks?date=2026-07-30"
			if query != "" {
				target += "&status=" + query
			}

			recorder := doRequest(t, srv, http.MethodGet, target, "")

			if recorder.Code != http.StatusOK {
				t.Fatalf("status = %d, want %d", recorder.Code, http.StatusOK)
			}
			if store.lastList.status != want {
				t.Errorf("status = %q, want %q", store.lastList.status, want)
			}
		})
	}
}

func TestListTasksPagesThroughACursor(t *testing.T) {
	t.Parallel()

	store := newFakeTaskStore()
	first := listedTask(t, "First", time.Date(2026, 7, 30, 0, 0, 0, 0, time.UTC))
	second := listedTask(t, "Second", time.Date(2026, 7, 30, 0, 0, 0, 0, time.UTC))
	store.listed = []task.Task{first, second}
	srv, _ := authedTaskServer(t, store)

	recorder := doRequest(t, srv, http.MethodGet, "/api/tasks?date=2026-07-30&limit=1", "")

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusOK)
	}
	got := decodeBody[taskListBody](t, recorder)
	if len(got.Tasks) != 1 || got.Tasks[0].Title != "First" {
		t.Fatalf("tasks = %+v, want only the first", got.Tasks)
	}
	if got.NextCursor == nil {
		t.Fatal("next_cursor = null, want a cursor")
	}
	if store.lastList.page.Limit != 2 {
		t.Errorf("store limit = %d, want the probe limit 2", store.lastList.page.Limit)
	}

	recorder = doRequest(t, srv, http.MethodGet, "/api/tasks?date=2026-07-30&limit=1&cursor="+*got.NextCursor, "")

	if recorder.Code != http.StatusOK {
		t.Fatalf("second page status = %d, want %d", recorder.Code, http.StatusOK)
	}
	if store.lastList.page.AfterID != first.ID {
		t.Errorf("cursor id = %v, want %v", store.lastList.page.AfterID, first.ID)
	}
	if !store.lastList.page.AfterDueOn.Equal(first.DueOn) {
		t.Errorf("cursor date = %v, want %v", store.lastList.page.AfterDueOn, first.DueOn)
	}
}

func TestListTasksEndsTheWalkOnAFullFinalPage(t *testing.T) {
	t.Parallel()

	store := newFakeTaskStore()
	store.listed = []task.Task{
		listedTask(t, "First", time.Date(2026, 7, 30, 0, 0, 0, 0, time.UTC)),
		listedTask(t, "Second", time.Date(2026, 7, 30, 0, 0, 0, 0, time.UTC)),
	}
	srv, _ := authedTaskServer(t, store)

	recorder := doRequest(t, srv, http.MethodGet, "/api/tasks?date=2026-07-30&limit=2", "")

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusOK)
	}
	got := decodeBody[taskListBody](t, recorder)
	if len(got.Tasks) != 2 {
		t.Fatalf("tasks = %d, want 2", len(got.Tasks))
	}
	if got.NextCursor != nil {
		t.Errorf("next_cursor = %q, want null when the page is full but nothing follows", *got.NextCursor)
	}
}

func TestListTasksAlwaysRendersAnArray(t *testing.T) {
	t.Parallel()

	srv, _ := authedTaskServer(t, newFakeTaskStore())

	recorder := doRequest(t, srv, http.MethodGet, "/api/tasks?date=2026-07-30", "")

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusOK)
	}
	var envelope map[string]json.RawMessage
	if err := json.Unmarshal(recorder.Body.Bytes(), &envelope); err != nil {
		t.Fatalf("decoding response: %v", err)
	}
	if string(envelope["tasks"]) != "[]" {
		t.Errorf("tasks = %s, want []", envelope["tasks"])
	}
	if string(envelope["next_cursor"]) != "null" {
		t.Errorf("next_cursor = %s, want null", envelope["next_cursor"])
	}
}

func TestListTasksRejectsInvalidQueries(t *testing.T) {
	t.Parallel()

	staleCursor := base64.RawURLEncoding.EncodeToString([]byte(`{"due_on":"nope","id":"` +
		uuid.Must(uuid.NewV7()).String() + `"}`))
	tests := map[string]struct {
		query     string
		wantError string
	}{
		"no filter": {query: "", wantError: "one of date, due_before, or contact_id is required"},
		"date and due_before": {
			query:     "date=2026-07-30&due_before=2026-07-30",
			wantError: "one of date, due_before, or contact_id is required",
		},
		"date and contact": {
			query:     "date=2026-07-30&contact_id=" + uuid.Must(uuid.NewV7()).String(),
			wantError: "one of date, due_before, or contact_id is required",
		},
		"malformed date":   {query: "date=30-07-2026", wantError: "malformed date"},
		"malformed before": {query: "due_before=30-07-2026", wantError: "malformed date"},
		"malformed contact": {
			query:     "contact_id=nope",
			wantError: "malformed contact id",
		},
		"unknown status": {query: "date=2026-07-30&status=archived", wantError: "invalid status"},
		"limit zero":     {query: "date=2026-07-30&limit=0", wantError: "invalid limit"},
		"limit too big":  {query: "date=2026-07-30&limit=201", wantError: "invalid limit"},
		"limit garbage":  {query: "date=2026-07-30&limit=lots", wantError: "invalid limit"},
		"cursor bad base64": {
			query:     "date=2026-07-30&cursor=%21%21%21",
			wantError: "malformed cursor",
		},
		"cursor bad json": {
			query:     "date=2026-07-30&cursor=" + base64.RawURLEncoding.EncodeToString([]byte("not json")),
			wantError: "malformed cursor",
		},
		"cursor with a bad date": {
			query:     "date=2026-07-30&cursor=" + staleCursor,
			wantError: "malformed cursor",
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			srv, _ := authedTaskServer(t, newFakeTaskStore())

			recorder := doRequest(t, srv, http.MethodGet, "/api/tasks?"+tc.query, "")

			if recorder.Code != http.StatusBadRequest {
				t.Fatalf("status = %d, want %d", recorder.Code, http.StatusBadRequest)
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

func TestListTasksReportsStorageFailure(t *testing.T) {
	t.Parallel()

	store := newFakeTaskStore()
	store.listErr = errors.New("store down")
	srv, _ := authedTaskServer(t, store)

	for _, query := range []string{
		"date=2026-07-30",
		"due_before=2026-07-30",
		"contact_id=" + uuid.Must(uuid.NewV7()).String(),
	} {
		recorder := doRequest(t, srv, http.MethodGet, "/api/tasks?"+query, "")

		if recorder.Code != http.StatusInternalServerError {
			t.Errorf("%s status = %d, want %d", query, recorder.Code, http.StatusInternalServerError)
		}
	}
}

func TestListTasksRequiresASession(t *testing.T) {
	t.Parallel()

	users := newFakeUserStore()
	addAda(t, users)
	srv := unauthedTaskServer(users, newFakeTaskStore())

	recorder := doRequest(t, srv, http.MethodGet, "/api/tasks?date=2026-07-30", "")

	if recorder.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusUnauthorized)
	}
}
