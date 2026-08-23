// SPDX-License-Identifier: Elastic-2.0

package server_test

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gopherium/alphone/internal/role"
)

// graphResponse is a GraphQL response body with raw data and typed errors.
type graphResponse struct {
	Data   json.RawMessage `json:"data"`
	Errors []struct {
		Message    string         `json:"message"`
		Extensions map[string]any `json:"extensions"`
	} `json:"errors"`
}

// graphErrorCode returns the first error's extensions code.
func graphErrorCode(t *testing.T, recorder *httptest.ResponseRecorder) string {
	t.Helper()
	body := decodeBody[graphResponse](t, recorder)
	if len(body.Errors) == 0 {
		t.Fatalf("no errors in response: %s", recorder.Body.String())
	}
	code, _ := body.Errors[0].Extensions["code"].(string)
	return code
}

// newAuthGraphServer returns a server over the default test user standing in one tier.
func newAuthGraphServer(t *testing.T, tier role.Role) http.Handler {
	t.Helper()
	users := newFakeUserStore()
	ada := addAda(t, users)
	ada.Role = tier.String()
	users.Users[ada.ID] = ada
	return newGraphServer(t, graphConfig{
		Contacts: newFakeContactStore(),
		Tasks:    newFakeTaskStore(),
		Users:    users,
		Version:  "9.9.9",
	})
}

// graphBody wraps a query document into a GraphQL request body.
func graphBody(t *testing.T, query string) string {
	t.Helper()
	payload, err := json.Marshal(map[string]string{"query": query})
	if err != nil {
		t.Fatalf("encoding the request body: %v", err)
	}
	return string(payload)
}

// loginMutation posts a graph login for the given credentials.
func loginMutation(t *testing.T, srv http.Handler, email, password string) *httptest.ResponseRecorder {
	t.Helper()
	query := fmt.Sprintf(
		`mutation { login(email: %q, password: %q) { me { id email name } } }`, email, password,
	)
	return postGraphQL(t, srv, graphBody(t, query), nil)
}

// graphSessionCookie returns the session cookie set by a login response.
func graphSessionCookie(t *testing.T, recorder *httptest.ResponseRecorder) *http.Cookie {
	t.Helper()
	for _, cookie := range recorder.Result().Cookies() {
		if cookie.Name == "__Host-alphone_session" {
			return cookie
		}
	}
	t.Fatalf("no session cookie in the response: %v", recorder.Header())
	return nil
}

func TestGraphLoginIssuesASessionForMe(t *testing.T) {
	t.Parallel()

	srv := newAuthGraphServer(t, role.Member)

	recorder := loginMutation(t, srv, "ada@example.com", testPassword)

	if recorder.Code != http.StatusOK {
		t.Fatalf("login status = %d, want %d: %s", recorder.Code, http.StatusOK, recorder.Body.String())
	}
	var payload struct {
		Login struct {
			Me struct {
				Email string `json:"email"`
			} `json:"me"`
		} `json:"login"`
	}
	body := decodeBody[graphResponse](t, recorder)
	if len(body.Errors) != 0 {
		t.Fatalf("login errors = %v, want none", body.Errors)
	}
	if err := json.Unmarshal(body.Data, &payload); err != nil {
		t.Fatalf("decoding login data: %v", err)
	}
	if payload.Login.Me.Email != "ada@example.com" {
		t.Errorf("me.email = %q, want ada@example.com", payload.Login.Me.Email)
	}
	cookie := graphSessionCookie(t, recorder)
	if !cookie.HttpOnly || !cookie.Secure || cookie.Value == "" {
		t.Errorf("session cookie = %+v, want a populated HttpOnly Secure cookie", cookie)
	}

	me := postGraphQL(t, srv, `{"query":"{ me { email } }"}`, cookie)
	if !strings.Contains(me.Body.String(), "ada@example.com") {
		t.Errorf("me with the issued cookie = %s, want ada's identity", me.Body.String())
	}
}

func TestGraphAnonymousCallersReachOnlyLogin(t *testing.T) {
	t.Parallel()

	srv := newAuthGraphServer(t, role.Member)
	blockedOperations := map[string]string{
		"me query":      `{ me { email } }`,
		"version query": `{ version }`,
		"introspection": `{ __schema { queryType { name } } }`,
		"fragment trick": `mutation { ...f } fragment f on Mutation {` +
			` login(email: "a@example.com", password: "x") { me { id } } }`,
	}

	for name, operation := range blockedOperations {
		recorder := postGraphQL(t, srv, graphBody(t, operation), nil)
		if got := graphErrorCode(t, recorder); got != "UNAUTHENTICATED" {
			t.Errorf("%s code = %q, want UNAUTHENTICATED", name, got)
		}
	}
}

func TestGraphLoginRateLimitsByClientIP(t *testing.T) {
	t.Parallel()

	srv := newAuthGraphServer(t, role.Member)

	for range 10 {
		recorder := loginMutation(t, srv, "nobody@example.com", "wrong password")
		if got := graphErrorCode(t, recorder); got != "UNAUTHENTICATED" {
			t.Fatalf("failed login code = %q, want UNAUTHENTICATED", got)
		}
	}

	blocked := loginMutation(t, srv, "ada@example.com", testPassword)
	body := decodeBody[graphResponse](t, blocked)
	if len(body.Errors) == 0 {
		t.Fatalf("blocked login answered no errors: %s", blocked.Body.String())
	}
	if code, _ := body.Errors[0].Extensions["code"].(string); code != "RATE_LIMITED" {
		t.Errorf("blocked code = %q, want RATE_LIMITED", code)
	}
	if retryAfter, _ := body.Errors[0].Extensions["retryAfter"].(float64); retryAfter != 120 {
		t.Errorf("retryAfter = %v, want the login limiter's 120 seconds", body.Errors[0].Extensions["retryAfter"])
	}
}

func TestGraphBearerCallersPassTheGate(t *testing.T) {
	t.Parallel()

	handler, _, _, secret := newTokenServer(t)
	request := httptest.NewRequest(http.MethodPost, "/api/graphql", strings.NewReader(`{"query":"{ me { email } }"}`))
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Authorization", "Bearer "+secret)
	recorder := httptest.NewRecorder()

	handler.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK || !strings.Contains(recorder.Body.String(), "ada@example.com") {
		t.Errorf("bearer me = %d %s, want ada's identity", recorder.Code, recorder.Body.String())
	}
}

func TestGraphLogoutClearsTheSession(t *testing.T) {
	t.Parallel()

	srv := newAuthGraphServer(t, role.Member)
	cookie := graphSessionCookie(t, loginMutation(t, srv, "ada@example.com", testPassword))

	recorder := postGraphQL(t, srv, `{"query":"mutation { logout }"}`, cookie)

	if !strings.Contains(recorder.Body.String(), `"logout":true`) {
		t.Fatalf("logout = %s, want true", recorder.Body.String())
	}
	cleared := graphSessionCookie(t, recorder)
	if cleared.Value != "" || cleared.MaxAge >= 0 {
		t.Errorf("cleared cookie = %+v, want an emptied expiring cookie", cleared)
	}

	me := postGraphQL(t, srv, `{"query":"{ me { email } }"}`, cookie)
	if got := graphErrorCode(t, me); got != "UNAUTHENTICATED" {
		t.Errorf("me after logout code = %q, want UNAUTHENTICATED", got)
	}
}

func TestGraphUserAdministration(t *testing.T) {
	t.Parallel()

	srv := newAuthGraphServer(t, role.Admin)
	cookie := graphSessionCookie(t, loginMutation(t, srv, "ada@example.com", testPassword))

	created := postGraphQL(t, srv, graphBody(t, `mutation { createUser(`+
		`email: "maria@example.com", name: "Maria Perez", password: "password1234")`+
		` { id email disabled } }`), cookie)
	body := decodeBody[graphResponse](t, created)
	if len(body.Errors) != 0 {
		t.Fatalf("createUser errors = %v, want none", body.Errors)
	}
	var createdPayload struct {
		CreateUser struct {
			ID       string `json:"id"`
			Disabled bool   `json:"disabled"`
		} `json:"createUser"`
	}
	if err := json.Unmarshal(body.Data, &createdPayload); err != nil {
		t.Fatalf("decoding createUser: %v", err)
	}

	listed := postGraphQL(t, srv, `{"query":"{ users { email } }"}`, cookie)
	if !strings.Contains(listed.Body.String(), "maria@example.com") {
		t.Errorf("users = %s, want maria listed", listed.Body.String())
	}

	disabled := postGraphQL(t, srv, graphBody(t, fmt.Sprintf(
		`mutation { setUserDisabled(id: %q, disabled: true) }`, createdPayload.CreateUser.ID,
	)), cookie)
	if !strings.Contains(disabled.Body.String(), `"setUserDisabled":true`) {
		t.Errorf("setUserDisabled = %s, want true", disabled.Body.String())
	}

	duplicate := postGraphQL(t, srv, graphBody(t, `mutation { createUser(`+
		`email: "maria@example.com", name: "Maria Perez", password: "password1234") { id } }`), cookie)
	if got := graphErrorCode(t, duplicate); got != "CONFLICT" {
		t.Errorf("duplicate email code = %q, want CONFLICT", got)
	}
}

func TestGraphRefusesUserManagementToAMemberSession(t *testing.T) {
	t.Parallel()

	srv := newAuthGraphServer(t, role.Member)
	cookie := graphSessionCookie(t, loginMutation(t, srv, "ada@example.com", testPassword))

	recorder := postGraphQL(t, srv, graphBody(t, `mutation { createUser(`+
		`email: "maria@example.com", name: "Maria Perez", password: "password1234") { id } }`), cookie)

	body := decodeBody[graphResponse](t, recorder)
	if len(body.Errors) != 1 {
		t.Fatalf("errors = %v, want one error, a member does not manage users", body.Errors)
	}
	if got, want := body.Errors[0].Message, "admin required"; got != want {
		t.Errorf("message = %q, want %q", got, want)
	}
	if got, want := body.Errors[0].Extensions["scope"], "users:write"; got != want {
		t.Errorf("scope = %v, want %q", got, want)
	}
}

func TestGraphLetsAMemberSessionListTheUsers(t *testing.T) {
	t.Parallel()

	srv := newAuthGraphServer(t, role.Member)
	cookie := graphSessionCookie(t, loginMutation(t, srv, "ada@example.com", testPassword))

	recorder := postGraphQL(t, srv, `{"query":"{ users { email } }"}`, cookie)

	if !strings.Contains(recorder.Body.String(), "ada@example.com") {
		t.Errorf("users = %s, want a member reading its colleagues", recorder.Body.String())
	}
}

func TestGraphSelfDisableIsRejected(t *testing.T) {
	t.Parallel()

	srv := newAuthGraphServer(t, role.Admin)
	login := loginMutation(t, srv, "ada@example.com", testPassword)
	cookie := graphSessionCookie(t, login)
	var payload struct {
		Login struct {
			Me struct {
				ID string `json:"id"`
			} `json:"me"`
		} `json:"login"`
	}
	if err := json.Unmarshal(decodeBody[graphResponse](t, login).Data, &payload); err != nil {
		t.Fatalf("decoding login: %v", err)
	}

	recorder := postGraphQL(t, srv, graphBody(t, fmt.Sprintf(
		`mutation { setUserDisabled(id: %q, disabled: true) }`, payload.Login.Me.ID,
	)), cookie)

	if got := graphErrorCode(t, recorder); got != "VALIDATION" {
		t.Errorf("self disable code = %q, want VALIDATION", got)
	}
}
