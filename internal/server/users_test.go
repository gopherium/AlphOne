// SPDX-License-Identifier: Elastic-2.0

package server_test

import (
	"errors"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/gopherium/gouncer"

	"github.com/gopherium/alphone/internal/postgres"
	"github.com/gopherium/alphone/internal/server"
)

var (
	_ server.UserStore = (*postgres.UserStore)(nil)
	_ server.UserStore = (*fakeUserStore)(nil)
)

type userBody struct {
	ID        uuid.UUID `json:"id"`
	Email     string    `json:"email"`
	Name      string    `json:"name"`
	Disabled  bool      `json:"disabled"`
	CreatedAt time.Time `json:"created_at"`
}

func authedUserServer(t *testing.T, users *fakeUserStore) (http.Handler, gouncer.User) {
	t.Helper()
	admin := users.addUser(t)
	srv := server.NewServer(server.Config{Contacts: newFakeContactStore(), Users: users})
	cookie := loginCookie(t, srv)
	authed := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		r.AddCookie(cookie)
		srv.ServeHTTP(w, r)
	})
	return authed, admin
}

func TestUserEndpointsRequireASession(t *testing.T) {
	t.Parallel()

	srv := server.NewServer(server.Config{Contacts: newFakeContactStore(), Users: newFakeUserStore()})

	for _, tc := range []struct{ method, target string }{
		{http.MethodGet, "/api/users"},
		{http.MethodPost, "/api/users"},
		{http.MethodPatch, "/api/users/" + uuid.Must(uuid.NewV7()).String()},
	} {
		recorder := doRequest(t, srv, tc.method, tc.target, "")
		if recorder.Code != http.StatusUnauthorized {
			t.Errorf("%s %s status = %d, want %d", tc.method, tc.target, recorder.Code, http.StatusUnauthorized)
		}
	}
}

func TestListUsers(t *testing.T) {
	t.Parallel()

	users := newFakeUserStore()
	srv, admin := authedUserServer(t, users)
	grace := users.addNamedUser(t, "grace@example.com", "Grace Hopper")

	recorder := doRequest(t, srv, http.MethodGet, "/api/users", "")

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d: %s", recorder.Code, http.StatusOK, recorder.Body.String())
	}
	got := decodeBody[[]userBody](t, recorder)
	if len(got) != 2 {
		t.Fatalf("listed %d users, want 2", len(got))
	}
	if got[0].ID != admin.ID || got[1].ID != grace.ID {
		t.Errorf("users listed as [%s, %s], want name order [%s, %s]", got[0].Name, got[1].Name, admin.Name, grace.Name)
	}
	if strings.Contains(strings.ToLower(recorder.Body.String()), "password") {
		t.Error("user listing leaks password material")
	}
}

func TestCreateUser(t *testing.T) {
	t.Parallel()

	users := newFakeUserStore()
	srv, _ := authedUserServer(t, users)

	recorder := doRequest(t, srv, http.MethodPost, "/api/users",
		`{"email":" Grace@Example.com ","name":"Grace Hopper","password":"correct horse battery"}`)

	if recorder.Code != http.StatusCreated {
		t.Fatalf("status = %d, want %d: %s", recorder.Code, http.StatusCreated, recorder.Body.String())
	}
	got := decodeBody[userBody](t, recorder)
	if got.Email != "grace@example.com" || got.Name != "Grace Hopper" || got.Disabled {
		t.Errorf("created user = %+v, want normalized email, given name, enabled", got)
	}
	stored, err := users.UserByEmail(t.Context(), "grace@example.com")
	if err != nil {
		t.Fatalf("created user not stored: %v", err)
	}
	if !gouncer.VerifyPassword(stored.PasswordHash, "correct horse battery") {
		t.Error("stored user does not verify against the given password")
	}
}

func TestCreateUserRejectsInvalidInput(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		body       string
		wantStatus int
	}{
		"malformed json": {"{", http.StatusBadRequest},
		"invalid email": {
			`{"email":"nope","name":"Grace","password":"correct horse battery"}`,
			http.StatusUnprocessableEntity,
		},
		"weak password": {
			`{"email":"grace@example.com","name":"Grace","password":"short"}`,
			http.StatusUnprocessableEntity,
		},
		"taken email": {
			`{"email":"ada@example.com","name":"Ada Again","password":"correct horse battery"}`,
			http.StatusConflict,
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			srv, _ := authedUserServer(t, newFakeUserStore())

			recorder := doRequest(t, srv, http.MethodPost, "/api/users", tc.body)

			if recorder.Code != tc.wantStatus {
				t.Errorf("status = %d, want %d: %s", recorder.Code, tc.wantStatus, recorder.Body.String())
			}
		})
	}
}

func TestSetUserDisabled(t *testing.T) {
	t.Parallel()

	users := newFakeUserStore()
	srv, _ := authedUserServer(t, users)
	grace := users.addNamedUser(t, "grace@example.com", "Grace Hopper")

	recorder := doRequest(t, srv, http.MethodPatch, "/api/users/"+grace.ID.String(), `{"disabled":true}`)

	if recorder.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want %d: %s", recorder.Code, http.StatusNoContent, recorder.Body.String())
	}
	if !users.users[grace.ID].Disabled {
		t.Error("user still enabled after the disable request")
	}

	recorder = doRequest(t, srv, http.MethodPatch, "/api/users/"+grace.ID.String(), `{"disabled":false}`)

	if recorder.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want %d: %s", recorder.Code, http.StatusNoContent, recorder.Body.String())
	}
	if users.users[grace.ID].Disabled {
		t.Error("user still disabled after the enable request")
	}
}

func TestSetUserDisabledRequiresTheDisabledField(t *testing.T) {
	t.Parallel()

	users := newFakeUserStore()
	srv, _ := authedUserServer(t, users)
	grace := users.addNamedUser(t, "grace@example.com", "Grace Hopper")
	if doRequest(t, srv, http.MethodPatch, "/api/users/"+grace.ID.String(),
		`{"disabled":true}`).Code != http.StatusNoContent {
		t.Fatal("precondition: could not disable the account")
	}

	recorder := doRequest(t, srv, http.MethodPatch, "/api/users/"+grace.ID.String(), `{}`)

	if recorder.Code != http.StatusUnprocessableEntity {
		t.Errorf("omitted disabled field status = %d, want %d", recorder.Code, http.StatusUnprocessableEntity)
	}
	if !users.users[grace.ID].Disabled {
		t.Error("an omitted disabled field silently re-enabled the disabled account")
	}
}

func TestSetUserDisabledGuardsTheOwnAccount(t *testing.T) {
	t.Parallel()

	users := newFakeUserStore()
	srv, admin := authedUserServer(t, users)

	recorder := doRequest(t, srv, http.MethodPatch, "/api/users/"+admin.ID.String(), `{"disabled":true}`)

	if recorder.Code != http.StatusUnprocessableEntity {
		t.Fatalf("status = %d, want %d: %s", recorder.Code, http.StatusUnprocessableEntity, recorder.Body.String())
	}
	if users.users[admin.ID].Disabled {
		t.Error("own account was disabled despite the guard")
	}

	recorder = doRequest(t, srv, http.MethodPatch, "/api/users/"+admin.ID.String(), `{"disabled":false}`)

	if recorder.Code != http.StatusNoContent {
		t.Errorf("re-enabling the own account status = %d, want %d", recorder.Code, http.StatusNoContent)
	}
}

func TestSetUserDisabledRejectsInvalidRequests(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		target     string
		body       string
		wantStatus int
	}{
		"malformed id":   {"/api/users/not-a-uuid", `{"disabled":true}`, http.StatusBadRequest},
		"unknown user":   {"/api/users/" + uuid.Must(uuid.NewV7()).String(), `{"disabled":true}`, http.StatusNotFound},
		"malformed body": {"/api/users/" + uuid.Must(uuid.NewV7()).String(), "{", http.StatusBadRequest},
		"misspelled field": {
			"/api/users/" + uuid.Must(uuid.NewV7()).String(), `{"disabld":true}`, http.StatusUnprocessableEntity,
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			srv, _ := authedUserServer(t, newFakeUserStore())

			recorder := doRequest(t, srv, http.MethodPatch, tc.target, tc.body)

			if recorder.Code != tc.wantStatus {
				t.Errorf("status = %d, want %d: %s", recorder.Code, tc.wantStatus, recorder.Body.String())
			}
		})
	}
}

func TestUserEndpointsReportBackendFailures(t *testing.T) {
	t.Parallel()

	users := newFakeUserStore()
	srv, _ := authedUserServer(t, users)
	users.listUsersErr = errors.New("list broke")
	users.createUserErr = errors.New("create broke")
	users.setDisabledErr = errors.New("disable broke")
	grace := users.addNamedUser(t, "grace@example.com", "Grace Hopper")

	for _, tc := range []struct{ method, target, body string }{
		{http.MethodGet, "/api/users", ""},
		{http.MethodPost, "/api/users", `{"email":"new@example.com","name":"New","password":"correct horse battery"}`},
		{http.MethodPatch, "/api/users/" + grace.ID.String(), `{"disabled":true}`},
	} {
		recorder := doRequest(t, srv, tc.method, tc.target, tc.body)
		if recorder.Code != http.StatusInternalServerError {
			t.Errorf("%s %s status = %d, want %d", tc.method, tc.target, recorder.Code, http.StatusInternalServerError)
		}
	}
}
