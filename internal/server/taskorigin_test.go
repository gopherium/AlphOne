// SPDX-License-Identifier: Elastic-2.0

package server_test

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/google/uuid"

	"github.com/gopherium/gouncer/authkit/testkit"

	"github.com/gopherium/alphone/internal/apitoken"
	"github.com/gopherium/alphone/internal/event"
	"github.com/gopherium/alphone/internal/server"
)

// mintFor stores a token of the given name for a user and returns its secret.
func mintFor(t *testing.T, tokens *fakeTokenStore, userID uuid.UUID, name string) string {
	t.Helper()
	minted, err := apitoken.Mint(userID, name)
	if err != nil {
		t.Fatalf("apitoken.Mint() error = %v, want nil", err)
	}
	tokens.tokens[minted.Token.Hash] = minted.Token
	return minted.Secret
}

// newOriginServer returns a server accepting bearer tokens and publishing to
// the returned recorder, with a secret minted for each named token.
func newOriginServer(t *testing.T, names ...string) (
	http.Handler, *fakePublisher, *fakeTaskStore, []string,
) {
	t.Helper()
	users := newFakeUserStore()
	ada := addAda(t, users)
	tokens := newFakeTokenStore()
	secrets := make([]string, len(names))
	for i, name := range names {
		secrets[i] = mintFor(t, tokens, ada.ID, name)
	}
	handler, tasks, events := originServerOver(users, tokens)
	return handler, events, tasks, secrets
}

// originServerOver returns a server reading the given users and tokens.
func originServerOver(users *testkit.Store, tokens *fakeTokenStore) (
	http.Handler, *fakeTaskStore, *fakePublisher,
) {
	tasks := newFakeTaskStore()
	events := &fakePublisher{}
	handler := server.NewServer(server.Config{
		Contacts: newFakeContactStore(),
		Tasks:    tasks,
		Users:    users,
		Tokens:   tokens,
		Events:   events,
	})
	return handler, tasks, events
}

// postTaskWithBearer creates a task with the given bearer credential.
func postTaskWithBearer(handler http.Handler, secret, body string) *httptest.ResponseRecorder {
	request := httptest.NewRequest(http.MethodPost, "/api/tasks", strings.NewReader(body))
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Authorization", "Bearer "+secret)
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)
	return recorder
}

func TestCreateTaskStampsACallerOriginEvent(t *testing.T) {
	t.Parallel()

	handler, _, _, secrets := newOriginServer(t, "n8n production")
	eventID := uuid.Must(uuid.NewV7())

	recorder := postTaskWithBearer(handler, secrets[0],
		`{"title":"Follow up with Maria Perez","due_on":"2026-08-01",`+
			`"origin_event_id":"`+eventID.String()+`"}`)

	if recorder.Code != http.StatusCreated {
		t.Fatalf("status = %d, want %d, body %s", recorder.Code, http.StatusCreated, recorder.Body)
	}
	got := decodeBody[taskBody](t, recorder)
	if got.OriginEventID == nil || *got.OriginEventID != eventID {
		t.Errorf("origin_event_id = %v, want %v", got.OriginEventID, eventID)
	}
	if got.OriginSource == nil || *got.OriginSource != "token:n8n production" {
		t.Errorf("origin_source = %v, want the stamped token name", got.OriginSource)
	}
}

func TestCreateTaskAnswersTheFirstTaskOnAReplay(t *testing.T) {
	t.Parallel()

	handler, events, tasks, secrets := newOriginServer(t, "n8n production")
	body := `{"title":"Follow up with Maria Perez","due_on":"2026-08-01",` +
		`"origin_event_id":"0198d000-0000-7000-8000-0000000000e1"}`

	first := postTaskWithBearer(handler, secrets[0], body)
	if first.Code != http.StatusCreated {
		t.Fatalf("first status = %d, want %d, body %s", first.Code, http.StatusCreated, first.Body)
	}
	replay := postTaskWithBearer(handler, secrets[0], body)

	if replay.Code != http.StatusOK {
		t.Fatalf("replay status = %d, want %d, body %s", replay.Code, http.StatusOK, replay.Body)
	}
	created := decodeBody[taskBody](t, first)
	replayed := decodeBody[taskBody](t, replay)
	if replayed.ID != created.ID {
		t.Errorf("replay id = %v, want the first task %v", replayed.ID, created.ID)
	}
	if len(tasks.tasks) != 1 {
		t.Errorf("stored %d tasks, want 1", len(tasks.tasks))
	}
	if !events.sawOnly(event.TaskCreated) {
		t.Errorf("published %v, want %q exactly once", events.names(), event.TaskCreated)
	}
}

func TestCreateTaskKeepsTheSameEventApartPerToken(t *testing.T) {
	t.Parallel()

	handler, events, tasks, secrets := newOriginServer(t, "n8n production", "n8n staging")
	body := `{"title":"Follow up with Maria Perez","due_on":"2026-08-01",` +
		`"origin_event_id":"0198d000-0000-7000-8000-0000000000e1"}`

	production := postTaskWithBearer(handler, secrets[0], body)
	staging := postTaskWithBearer(handler, secrets[1], body)

	for name, recorder := range map[string]*httptest.ResponseRecorder{
		"production": production, "staging": staging,
	} {
		if recorder.Code != http.StatusCreated {
			t.Fatalf("%s status = %d, want %d, body %s",
				name, recorder.Code, http.StatusCreated, recorder.Body)
		}
	}
	if decodeBody[taskBody](t, production).ID == decodeBody[taskBody](t, staging).ID {
		t.Error("both tokens answered the same task, want one task each")
	}
	if len(tasks.tasks) != 2 {
		t.Errorf("stored %d tasks, want 2", len(tasks.tasks))
	}
	if len(events.names()) != 2 {
		t.Errorf("published %v, want two creations", events.names())
	}
}

func TestCreateTaskKeepsOwnersApartUnderOneTokenName(t *testing.T) {
	t.Parallel()

	users := newFakeUserStore()
	ada := addAda(t, users)
	grace := users.AddUser(t, "grace@example.com", "Grace Hopper", testPassword)
	tokens := newFakeTokenStore()
	adaSecret := mintFor(t, tokens, ada.ID, "n8n production")
	graceSecret := mintFor(t, tokens, grace.ID, "n8n production")
	handler, tasks, _ := originServerOver(users, tokens)
	body := `{"title":"Follow up with Maria Perez","due_on":"2026-08-01",` +
		`"origin_event_id":"0198d000-0000-7000-8000-0000000000e1"}`

	adas := postTaskWithBearer(handler, adaSecret, body)
	graces := postTaskWithBearer(handler, graceSecret, body)

	if adas.Code != http.StatusCreated || graces.Code != http.StatusCreated {
		t.Fatalf("statuses = %d and %d, want %d each",
			adas.Code, graces.Code, http.StatusCreated)
	}
	if decodeBody[taskBody](t, adas).ID == decodeBody[taskBody](t, graces).ID {
		t.Error("one owner answered for the other, want a task each")
	}
	if got := decodeBody[taskBody](t, graces).AssigneeID; got != grace.ID {
		t.Errorf("assignee_id = %v, want the second owner %v", got, grace.ID)
	}
	if len(tasks.tasks) != 2 {
		t.Errorf("stored %d tasks, want 2", len(tasks.tasks))
	}
}

func TestCreateTaskRejectsAnOriginEventFromASession(t *testing.T) {
	t.Parallel()

	store := newFakeTaskStore()
	srv, _ := authedTaskServer(t, store)

	recorder := doRequest(t, srv, http.MethodPost, "/api/tasks",
		`{"title":"Follow up with Maria Perez","due_on":"2026-08-01",`+
			`"origin_event_id":"0198d000-0000-7000-8000-0000000000e1"}`)

	if recorder.Code != http.StatusUnprocessableEntity {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusUnprocessableEntity)
	}
	if len(store.tasks) != 0 {
		t.Errorf("stored %d tasks, want none", len(store.tasks))
	}
}
