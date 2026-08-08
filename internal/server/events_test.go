// SPDX-License-Identifier: Elastic-2.0

package server_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/google/uuid"

	"github.com/gopherium/alphone/internal/event"
	"github.com/gopherium/alphone/internal/server"
	"github.com/gopherium/alphone/internal/task"
)

type publishedEvent struct {
	Name     event.Name
	Audience uuid.UUID
	Data     map[string]any
}

type fakePublisher struct {
	mu        sync.Mutex
	published []publishedEvent
}

func (p *fakePublisher) Publish(_ context.Context, frame event.Frame, data map[string]any) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.published = append(p.published, publishedEvent{Name: frame.Name, Audience: frame.Audience, Data: data})
}

// names returns every published event name in order.
func (p *fakePublisher) names() []event.Name {
	p.mu.Lock()
	defer p.mu.Unlock()
	var seen []event.Name
	for _, e := range p.published {
		seen = append(seen, e.Name)
	}
	return seen
}

// published reports whether the named event was published.
func (p *fakePublisher) sawOnly(name event.Name) bool {
	seen := p.names()
	return len(seen) == 1 && seen[0] == name
}

// newEventingServer returns a server publishing to the returned recorder,
// with a session cookie for the default test user.
func newEventingServer(t *testing.T) (http.Handler, *fakePublisher, *fakeTaskStore, *http.Cookie) {
	t.Helper()
	users := newFakeUserStore()
	addAda(t, users)
	tasks := newFakeTaskStore()
	events := &fakePublisher{}
	handler := server.NewServer(server.Config{
		Contacts: newFakeContactStore(),
		Tasks:    tasks,
		Users:    users,
		Events:   events,
	})
	return handler, events, tasks, loginCookie(t, handler)
}

// doJSON issues an authenticated JSON request.
func doJSON(handler http.Handler, cookie *http.Cookie, method, path, body string) *httptest.ResponseRecorder {
	request := httptest.NewRequest(method, path, strings.NewReader(body))
	request.Header.Set("Content-Type", "application/json")
	request.AddCookie(cookie)
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)
	return recorder
}

// onlyTask returns the single task the store holds.
func onlyTask(t *testing.T, tasks *fakeTaskStore) task.Task {
	t.Helper()
	for _, stored := range tasks.tasks {
		return stored
	}
	t.Fatal("no task in the store, want the created one")
	return task.Task{}
}

func TestCreatingATaskPublishesTaskCreated(t *testing.T) {
	t.Parallel()

	handler, events, tasks, cookie := newEventingServer(t)

	recorder := doJSON(handler, cookie, http.MethodPost, "/api/tasks",
		`{"title":"Call Maria","due_on":"2026-07-30"}`)

	if recorder.Code != http.StatusCreated {
		t.Fatalf("status = %d, want %d, body %s", recorder.Code, http.StatusCreated, recorder.Body)
	}
	if !events.sawOnly(event.TaskCreated) {
		t.Fatalf("published %v, want only %q", events.names(), event.TaskCreated)
	}
	data := events.published[0].Data
	if data["title"] != "Call Maria" {
		t.Errorf("data.title = %v, want the task title", data["title"])
	}
	if data["id"] == nil {
		t.Error("data.id is absent, want the task identifier a consumer refetches with")
	}
	if got := events.published[0].Audience; got != onlyTask(t, tasks).AssigneeID {
		t.Errorf("audience = %s, want the assignee %s", got, onlyTask(t, tasks).AssigneeID)
	}
}

func TestFailingToCreateATaskPublishesNothing(t *testing.T) {
	t.Parallel()

	handler, events, _, cookie := newEventingServer(t)

	recorder := doJSON(handler, cookie, http.MethodPost, "/api/tasks", `{"title":"  ","due_on":"2026-07-30"}`)

	if recorder.Code < 400 {
		t.Fatalf("status = %d, want a client error", recorder.Code)
	}
	if len(events.names()) != 0 {
		t.Errorf("published %v for a rejected task, want nothing", events.names())
	}
}

func TestCompletingATaskPublishesTaskCompleted(t *testing.T) {
	t.Parallel()

	handler, events, tasks, cookie := newEventingServer(t)
	created := doJSON(handler, cookie, http.MethodPost, "/api/tasks",
		`{"title":"Call Maria","due_on":"2026-07-30"}`)
	if created.Code != http.StatusCreated {
		t.Fatalf("create status = %d, want %d", created.Code, http.StatusCreated)
	}
	id := onlyTaskID(t, tasks)
	events.published = nil

	recorder := doJSON(handler, cookie, http.MethodPatch, "/api/tasks/"+id.String(), `{"status":"done"}`)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body %s", recorder.Code, http.StatusOK, recorder.Body)
	}
	if !events.sawOnly(event.TaskCompleted) {
		t.Errorf("published %v, want only %q", events.names(), event.TaskCompleted)
	}
}

func TestReopeningATaskPublishesNothing(t *testing.T) {
	t.Parallel()

	handler, events, tasks, cookie := newEventingServer(t)
	if r := doJSON(handler, cookie, http.MethodPost, "/api/tasks",
		`{"title":"Call Maria","due_on":"2026-07-30"}`); r.Code != http.StatusCreated {
		t.Fatalf("create status = %d, want %d", r.Code, http.StatusCreated)
	}
	id := onlyTaskID(t, tasks)
	if r := doJSON(handler, cookie, http.MethodPatch, "/api/tasks/"+id.String(),
		`{"status":"done"}`); r.Code != http.StatusOK {
		t.Fatalf("complete status = %d, want %d", r.Code, http.StatusOK)
	}
	events.published = nil

	recorder := doJSON(handler, cookie, http.MethodPatch, "/api/tasks/"+id.String(), `{"status":"open"}`)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusOK)
	}
	if len(events.names()) != 0 {
		t.Errorf("published %v for a reopened task, want nothing", events.names())
	}
}

func TestRenamingATaskPublishesNothing(t *testing.T) {
	t.Parallel()

	handler, events, tasks, cookie := newEventingServer(t)
	if r := doJSON(handler, cookie, http.MethodPost, "/api/tasks",
		`{"title":"Call Maria","due_on":"2026-07-30"}`); r.Code != http.StatusCreated {
		t.Fatalf("create status = %d, want %d", r.Code, http.StatusCreated)
	}
	id := onlyTaskID(t, tasks)
	events.published = nil

	recorder := doJSON(handler, cookie, http.MethodPatch, "/api/tasks/"+id.String(), `{"title":"Call Ada"}`)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusOK)
	}
	if len(events.names()) != 0 {
		t.Errorf("published %v for a renamed task, want nothing", events.names())
	}
}

func TestCompletingAnAlreadyDoneTaskPublishesOnce(t *testing.T) {
	t.Parallel()

	handler, events, tasks, cookie := newEventingServer(t)
	if r := doJSON(handler, cookie, http.MethodPost, "/api/tasks",
		`{"title":"Call Maria","due_on":"2026-07-30"}`); r.Code != http.StatusCreated {
		t.Fatalf("create status = %d, want %d", r.Code, http.StatusCreated)
	}
	id := onlyTaskID(t, tasks)
	if r := doJSON(handler, cookie, http.MethodPatch, "/api/tasks/"+id.String(),
		`{"status":"done"}`); r.Code != http.StatusOK {
		t.Fatalf("complete status = %d, want %d", r.Code, http.StatusOK)
	}
	events.published = nil

	recorder := doJSON(handler, cookie, http.MethodPatch, "/api/tasks/"+id.String(), `{"status":"done"}`)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusOK)
	}
	if len(events.names()) != 0 {
		t.Errorf("published %v completing an already done task, want nothing", events.names())
	}
}

func TestCreatingAContactPublishesContactCreated(t *testing.T) {
	t.Parallel()

	handler, events, _, cookie := newEventingServer(t)

	recorder := doJSON(handler, cookie, http.MethodPost, "/api/contacts", `{"name":"Maria Perez"}`)

	if recorder.Code != http.StatusCreated {
		t.Fatalf("status = %d, want %d, body %s", recorder.Code, http.StatusCreated, recorder.Body)
	}
	if !events.sawOnly(event.ContactCreated) {
		t.Fatalf("published %v, want only %q", events.names(), event.ContactCreated)
	}
	if events.published[0].Data["name"] != "Maria Perez" {
		t.Errorf("data.name = %v, want the contact name", events.published[0].Data["name"])
	}
	if got := events.published[0].Audience; got != uuid.Nil {
		t.Errorf("audience = %s, want everyone", got)
	}
}

func TestAServerWithoutAPublisherStillWorks(t *testing.T) {
	t.Parallel()

	users := newFakeUserStore()
	addAda(t, users)
	handler := server.NewServer(server.Config{
		Contacts: newFakeContactStore(),
		Tasks:    newFakeTaskStore(),
		Users:    users,
	})
	cookie := loginCookie(t, handler)

	recorder := doJSON(handler, cookie, http.MethodPost, "/api/contacts", `{"name":"Maria Perez"}`)

	if recorder.Code != http.StatusCreated {
		t.Errorf("status = %d, want %d without a publisher configured", recorder.Code, http.StatusCreated)
	}
}

// onlyTaskID returns the identifier of the single stored task.
func onlyTaskID(t *testing.T, tasks *fakeTaskStore) uuid.UUID {
	t.Helper()
	var found []task.Task
	for _, stored := range tasks.tasks {
		found = append(found, stored)
	}
	if len(found) != 1 {
		t.Fatalf("stored %d tasks, want 1", len(found))
	}
	return found[0].ID
}
