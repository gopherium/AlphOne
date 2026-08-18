// SPDX-License-Identifier: Elastic-2.0

package graphres_test

import (
	"context"
	"errors"
	"net/http"
	"testing"
	"time"

	gqlclient "github.com/99designs/gqlgen/client"
	"github.com/google/uuid"

	"github.com/gopherium/gouncer/authkit"
	"github.com/gopherium/gouncer/authkit/ratelimit"
	"github.com/gopherium/gouncer/authkit/testkit"

	"github.com/gopherium/alphone/internal/graphres"
	"github.com/gopherium/alphone/internal/role"
	"github.com/gopherium/alphone/sdk"
)

const testPassword = "password1234"

// newAuthResolver returns a resolver whose auth seams serve store, every user standing as a member.
func newAuthResolver(store *testkit.Store) *graphres.Resolver {
	return &graphres.Resolver{
		Version:      "9.9.9",
		Auth:         authkit.New(authkit.Config{Store: store, CookieName: "alphone_session"}),
		Admin:        authkit.NewAdmin(store),
		Roles:        standingRoleStore{tier: role.Member},
		LoginLimiter: ratelimit.NewLimiter(ratelimit.Config{Limit: 2, Window: time.Minute}),
	}
}

// newHTTPGraphClient returns a test client carrying the HTTP pair, caller IP, and identity.
func newHTTPGraphClient(t *testing.T, resolver *graphres.Resolver, identity uuid.UUID) *gqlclient.Client {
	t.Helper()
	srv := newGraphHandler(t, resolver)
	wrapped := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()
		if identity != uuid.Nil {
			ctx = authkitIdentity(ctx, identity)
		}
		ctx = graphres.WithHTTP(ctx, w, r)
		ctx = graphres.WithClientIP(ctx, "192.0.2.10")
		ctx = sdk.WithRequestScope(ctx, sdk.NewRequestScope())
		srv.ServeHTTP(w, r.WithContext(ctx))
	})
	return gqlclient.New(wrapped)
}

// newAnonymousGraphClient returns a test client without any identity on the context.
func newAnonymousGraphClient(t *testing.T, resolver *graphres.Resolver) *gqlclient.Client {
	t.Helper()
	return newDecoratedGraphClient(t, resolver, func(ctx context.Context) context.Context {
		return ctx
	})
}

// loginQuery is the login mutation used across the auth tests.
const loginQuery = `mutation ($email: String!, $password: String!) {
	login(email: $email, password: $password) { me { id email name } }
}`

func TestVersionReportsTheConfiguredVersion(t *testing.T) {
	t.Parallel()

	client := newGraphClient(t, newAuthResolver(testkit.NewStore()), uuid.Must(uuid.NewV7()))

	var response struct {
		Version string `json:"version"`
	}
	client.MustPost(`{ version }`, &response)

	if response.Version != "9.9.9" {
		t.Errorf("version = %q, want 9.9.9", response.Version)
	}
}

func TestMeReportsTheCallingIdentity(t *testing.T) {
	t.Parallel()

	caller := uuid.Must(uuid.NewV7())
	client := newGraphClient(t, newAuthResolver(testkit.NewStore()), caller)

	var response struct {
		Me struct {
			ID string `json:"id"`
		} `json:"me"`
	}
	client.MustPost(`{ me { id } }`, &response)

	if response.Me.ID != caller.String() {
		t.Errorf("me.id = %q, want %q", response.Me.ID, caller)
	}
}

func TestAnonymousOperationsBeyondLoginAreRejected(t *testing.T) {
	t.Parallel()

	client := newAnonymousGraphClient(t, newAuthResolver(testkit.NewStore()))

	for name, query := range map[string]string{
		"version query":       `{ version }`,
		"logout mutation":     `mutation { logout }`,
		"login beside logout": `mutation { login(email: "e", password: "p") { me { id } } logout }`,
		"me query":            `{ me { id } }`,
	} {
		response, err := client.RawPost(query)
		if err != nil {
			t.Fatalf("%s RawPost() error = %v, want nil", name, err)
		}
		if got := firstErrorCode(t, response.Errors); got != "UNAUTHENTICATED" {
			t.Errorf("%s code = %q, want UNAUTHENTICATED", name, got)
		}
	}
}

func TestLoginIssuesTheSessionForValidCredentials(t *testing.T) {
	t.Parallel()

	store := testkit.NewStore()
	maria := store.AddUser(t, "maria@example.com", "Maria Perez", testPassword)
	client := newHTTPGraphClient(t, newAuthResolver(store), uuid.Nil)

	var response struct {
		Login struct {
			Me struct {
				ID    string `json:"id"`
				Email string `json:"email"`
				Name  string `json:"name"`
			} `json:"me"`
		} `json:"login"`
	}
	client.MustPost(loginQuery, &response,
		gqlclient.Var("email", "maria@example.com"), gqlclient.Var("password", testPassword))

	if response.Login.Me.ID != maria.ID.String() || response.Login.Me.Name != "Maria Perez" {
		t.Errorf("login.me = %+v, want Maria Perez's identity", response.Login.Me)
	}
}

func TestLoginRejectsInvalidCredentials(t *testing.T) {
	t.Parallel()

	store := testkit.NewStore()
	store.AddUser(t, "maria@example.com", "Maria Perez", testPassword)
	client := newHTTPGraphClient(t, newAuthResolver(store), uuid.Nil)

	response, err := client.RawPost(loginQuery,
		gqlclient.Var("email", "maria@example.com"), gqlclient.Var("password", "wrong password 1234"))
	if err != nil {
		t.Fatalf("RawPost() error = %v, want nil", err)
	}

	if got := firstErrorCode(t, response.Errors); got != "UNAUTHENTICATED" {
		t.Errorf("code = %q, want UNAUTHENTICATED", got)
	}
}

func TestLoginRateLimitsRepeatedFailures(t *testing.T) {
	t.Parallel()

	store := testkit.NewStore()
	store.AddUser(t, "maria@example.com", "Maria Perez", testPassword)
	client := newHTTPGraphClient(t, newAuthResolver(store), uuid.Nil)
	wrong := []gqlclient.Option{
		gqlclient.Var("email", "maria@example.com"), gqlclient.Var("password", "wrong password 1234"),
	}

	for range 2 {
		if _, err := client.RawPost(loginQuery, wrong...); err != nil {
			t.Fatalf("RawPost() error = %v, want nil", err)
		}
	}
	blocked, err := client.RawPost(loginQuery,
		gqlclient.Var("email", "maria@example.com"), gqlclient.Var("password", testPassword))
	if err != nil {
		t.Fatalf("RawPost() error = %v, want nil", err)
	}

	if got := firstErrorCode(t, blocked.Errors); got != "RATE_LIMITED" {
		t.Errorf("code = %q, want RATE_LIMITED", got)
	}
}

// stubLimiter answers the login budget from fixed error fields.
type stubLimiter struct {
	checkErr  error
	recordErr error
}

func (s stubLimiter) Check(string) (bool, time.Duration, error) {
	return true, 0, s.checkErr
}

func (s stubLimiter) RecordFailure(string) error {
	return s.recordErr
}

func TestLoginSurfacesLimiterFailures(t *testing.T) {
	t.Parallel()

	store := testkit.NewStore()
	store.AddUser(t, "maria@example.com", "Maria Perez", testPassword)

	checkFailing := newAuthResolver(store)
	checkFailing.LoginLimiter = stubLimiter{checkErr: errors.New("limiter down")}
	client := newHTTPGraphClient(t, checkFailing, uuid.Nil)
	blocked, err := client.RawPost(loginQuery,
		gqlclient.Var("email", "maria@example.com"), gqlclient.Var("password", testPassword))
	if err != nil {
		t.Fatalf("RawPost() error = %v, want nil", err)
	}
	if got := firstErrorCode(t, blocked.Errors); got != "INTERNAL" {
		t.Errorf("failing check code = %q, want INTERNAL", got)
	}

	recordFailing := newAuthResolver(store)
	recordFailing.LoginLimiter = stubLimiter{recordErr: errors.New("limiter down")}
	recordClient := newHTTPGraphClient(t, recordFailing, uuid.Nil)
	unrecorded, err := recordClient.RawPost(loginQuery,
		gqlclient.Var("email", "maria@example.com"), gqlclient.Var("password", "wrong password 1234"))
	if err != nil {
		t.Fatalf("RawPost() error = %v, want nil", err)
	}
	if got := firstErrorCode(t, unrecorded.Errors); got != "INTERNAL" {
		t.Errorf("failing record code = %q, want INTERNAL", got)
	}
}

func TestLoginSurfacesSessionStoreFailures(t *testing.T) {
	t.Parallel()

	store := testkit.NewStore()
	store.AddUser(t, "maria@example.com", "Maria Perez", testPassword)
	store.CreateSessionErr = errors.New("session store down")
	client := newHTTPGraphClient(t, newAuthResolver(store), uuid.Nil)

	response, err := client.RawPost(loginQuery,
		gqlclient.Var("email", "maria@example.com"), gqlclient.Var("password", testPassword))
	if err != nil {
		t.Fatalf("RawPost() error = %v, want nil", err)
	}

	if got := firstErrorCode(t, response.Errors); got != "INTERNAL" {
		t.Errorf("code = %q, want INTERNAL", got)
	}
}

func TestLoginFailsWithoutTheHTTPTransport(t *testing.T) {
	t.Parallel()

	store := testkit.NewStore()
	store.AddUser(t, "maria@example.com", "Maria Perez", testPassword)
	client := newAnonymousGraphClient(t, newAuthResolver(store))

	response, err := client.RawPost(loginQuery,
		gqlclient.Var("email", "maria@example.com"), gqlclient.Var("password", testPassword))
	if err != nil {
		t.Fatalf("RawPost() error = %v, want nil", err)
	}

	if got := firstErrorCode(t, response.Errors); got != "INTERNAL" {
		t.Errorf("code = %q, want INTERNAL", got)
	}
}

func TestLogoutClearsTheSession(t *testing.T) {
	t.Parallel()

	client := newHTTPGraphClient(t, newAuthResolver(testkit.NewStore()), uuid.Must(uuid.NewV7()))

	var response struct {
		Logout bool `json:"logout"`
	}
	client.MustPost(`mutation { logout }`, &response)

	if !response.Logout {
		t.Error("logout = false, want true")
	}
}

func TestLogoutSurfacesSessionStoreFailures(t *testing.T) {
	t.Parallel()

	store := testkit.NewStore()
	store.DeleteErr = errors.New("session store down")
	client := newHTTPGraphClient(t, newAuthResolver(store), uuid.Must(uuid.NewV7()))

	response, err := client.RawPost(`mutation { logout }`,
		gqlclient.AddCookie(&http.Cookie{Name: "alphone_session", Value: "stale-token"}))
	if err != nil {
		t.Fatalf("RawPost() error = %v, want nil", err)
	}

	if got := firstErrorCode(t, response.Errors); got != "INTERNAL" {
		t.Errorf("code = %q, want INTERNAL", got)
	}
}

func TestLogoutFailsWithoutTheHTTPTransport(t *testing.T) {
	t.Parallel()

	client := newGraphClient(t, newAuthResolver(testkit.NewStore()), uuid.Must(uuid.NewV7()))

	response, err := client.RawPost(`mutation { logout }`)
	if err != nil {
		t.Fatalf("RawPost() error = %v, want nil", err)
	}

	if got := firstErrorCode(t, response.Errors); got != "INTERNAL" {
		t.Errorf("code = %q, want INTERNAL", got)
	}
}

func TestUsersListsEveryAccount(t *testing.T) {
	t.Parallel()

	store := testkit.NewStore()
	maria := store.AddUser(t, "maria@example.com", "Maria Perez", testPassword)
	client := newGraphClient(t, newAuthResolver(store), maria.ID)

	var response struct {
		Users []struct {
			ID        string `json:"id"`
			Email     string `json:"email"`
			Name      string `json:"name"`
			Disabled  bool   `json:"disabled"`
			CreatedAt string `json:"createdAt"`
		} `json:"users"`
	}
	client.MustPost(`{ users { id email name disabled createdAt } }`, &response)

	if len(response.Users) != 1 || response.Users[0].Email != "maria@example.com" {
		t.Errorf("users = %+v, want Maria Perez's account", response.Users)
	}
}

func TestUsersSurfacesStoreFailures(t *testing.T) {
	t.Parallel()

	store := testkit.NewStore()
	store.ListUsersErr = errors.New("user store down")
	client := newGraphClient(t, newAuthResolver(store), uuid.Must(uuid.NewV7()))

	response, err := client.RawPost(`{ users { id } }`)
	if err != nil {
		t.Fatalf("RawPost() error = %v, want nil", err)
	}

	if got := firstErrorCode(t, response.Errors); got != "INTERNAL" {
		t.Errorf("code = %q, want INTERNAL", got)
	}
}

func TestCreateUserStoresTheAccount(t *testing.T) {
	t.Parallel()

	client := newGraphClient(t, newAuthResolver(testkit.NewStore()), uuid.Must(uuid.NewV7()))

	var response struct {
		CreateUser struct {
			Email    string `json:"email"`
			Name     string `json:"name"`
			Disabled bool   `json:"disabled"`
		} `json:"createUser"`
	}
	client.MustPost(
		`mutation { createUser(email: "maria@example.com", name: "Maria Perez", password: "password1234") {
			email name disabled
		} }`,
		&response,
	)

	if response.CreateUser.Email != "maria@example.com" || response.CreateUser.Disabled {
		t.Errorf("createUser = %+v, want Maria Perez's enabled account", response.CreateUser)
	}
}

func TestCreateUserMapsTheAdminFailures(t *testing.T) {
	t.Parallel()

	store := testkit.NewStore()
	store.AddUser(t, "maria@example.com", "Maria Perez", testPassword)
	client := newGraphClient(t, newAuthResolver(store), uuid.Must(uuid.NewV7()))

	taken, err := client.RawPost(
		`mutation { createUser(email: "maria@example.com", name: "Maria Perez", password: "password1234") { id } }`,
	)
	if err != nil {
		t.Fatalf("RawPost() error = %v, want nil", err)
	}
	if got := firstErrorCode(t, taken.Errors); got != "CONFLICT" {
		t.Errorf("taken email code = %q, want CONFLICT", got)
	}

	weak, err := client.RawPost(
		`mutation { createUser(email: "new@example.com", name: "Maria Perez", password: "short") { id } }`,
	)
	if err != nil {
		t.Fatalf("RawPost() error = %v, want nil", err)
	}
	if got := firstErrorCode(t, weak.Errors); got != "VALIDATION" {
		t.Errorf("weak password code = %q, want VALIDATION", got)
	}
}

func TestSetUserDisabledUpdatesTheAccount(t *testing.T) {
	t.Parallel()

	store := testkit.NewStore()
	actor := store.AddUser(t, "maria@example.com", "Maria Perez", testPassword)
	target := store.AddUser(t, "second@example.com", "Second Account", testPassword)
	client := newGraphClient(t, newAuthResolver(store), actor.ID)

	var response struct {
		SetUserDisabled bool `json:"setUserDisabled"`
	}
	client.MustPost(`mutation ($id: UUID!) { setUserDisabled(id: $id, disabled: true) }`, &response,
		gqlclient.Var("id", target.ID))

	if !response.SetUserDisabled {
		t.Error("setUserDisabled = false, want true")
	}
}

func TestSetUserDisabledMapsTheAdminFailures(t *testing.T) {
	t.Parallel()

	store := testkit.NewStore()
	actor := store.AddUser(t, "maria@example.com", "Maria Perez", testPassword)
	client := newGraphClient(t, newAuthResolver(store), actor.ID)

	self, err := client.RawPost(`mutation ($id: UUID!) { setUserDisabled(id: $id, disabled: true) }`,
		gqlclient.Var("id", actor.ID))
	if err != nil {
		t.Fatalf("RawPost() error = %v, want nil", err)
	}
	if got := firstErrorCode(t, self.Errors); got != "VALIDATION" {
		t.Errorf("self disable code = %q, want VALIDATION", got)
	}

	missing, err := client.RawPost(`mutation ($id: UUID!) { setUserDisabled(id: $id, disabled: true) }`,
		gqlclient.Var("id", uuid.Must(uuid.NewV7())))
	if err != nil {
		t.Fatalf("RawPost() error = %v, want nil", err)
	}
	if got := firstErrorCode(t, missing.Errors); got != "NOT_FOUND" {
		t.Errorf("missing user code = %q, want NOT_FOUND", got)
	}
}
