// SPDX-License-Identifier: Elastic-2.0

package main

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/peterldowns/pgtestdb"

	"github.com/gopherium/gouncer"

	"github.com/gopherium/alphone/internal/contact"
	"github.com/gopherium/alphone/internal/postgres"
	"github.com/gopherium/alphone/internal/testdb"
	"github.com/gopherium/alphone/sdk"
)

func TestResolverBridgePropagatesFailures(t *testing.T) {
	t.Parallel()

	bridge := resolverBridge{resolver: contact.NewResolver(nil)}

	owner, err := bridge.Resolve(t.Context(), " \t ", "184467235@lid", "María")

	if !errors.Is(err, contact.ErrEmptyChannel) {
		t.Fatalf("Resolve() error = %v, want %v", err, contact.ErrEmptyChannel)
	}
	if owner != (sdk.Contact{}) {
		t.Errorf("Resolve() contact = %+v, want zero value on failure", owner)
	}
}

var errPluginMigrate = errors.New("plugin migration exploded")

type failingPlugin struct{}

func (failingPlugin) ID() string {
	return "failing"
}

func (failingPlugin) Start(_ context.Context) error {
	return nil
}

func (failingPlugin) Stop(_ context.Context) error {
	return nil
}

func (failingPlugin) Migrate(_ context.Context) error {
	return errPluginMigrate
}

func failingPlugins(_ sdk.Deps) ([]sdk.Plugin, error) {
	return []sdk.Plugin{failingPlugin{}}, nil
}

func testGetenv(values map[string]string) func(string) string {
	return func(key string) string {
		return values[key]
	}
}

func testDatabaseURL(t *testing.T) string {
	t.Helper()
	if testing.Short() {
		t.Skip("skipping database test in short mode")
	}
	cfg := pgtestdb.Custom(t, testdb.Config(), testdb.CoreMigrator())
	return cfg.URL()
}

func freeAddr(t *testing.T) string {
	t.Helper()
	listener, err := net.Listen("tcp", "localhost:0")
	if err != nil {
		t.Fatalf("finding a free port: %v", err)
	}
	addr := listener.Addr().String()
	if err := listener.Close(); err != nil {
		t.Fatalf("releasing the port: %v", err)
	}
	return addr
}

func waitForServer(t *testing.T, url string) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		response, err := http.Get(url)
		if err == nil {
			ready := response.StatusCode == http.StatusUnauthorized
			_ = response.Body.Close()
			if ready {
				return
			}
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatal("server never became ready")
}

func TestRunRequiresDatabaseURL(t *testing.T) {
	t.Parallel()

	err := run(t.Context(), testGetenv(nil), io.Discard, registerPlugins)

	if err == nil {
		t.Fatal("run() error = nil, want a configuration error")
	}
}

func TestRunReportsPluginFailure(t *testing.T) {
	t.Parallel()

	err := run(t.Context(), testGetenv(map[string]string{
		"ALPHONE_DATABASE_URL": testDatabaseURL(t),
	}), io.Discard, failingPlugins)

	if !errors.Is(err, errPluginMigrate) {
		t.Fatalf("run() error = %v, want %v in its chain", err, errPluginMigrate)
	}
}

func TestRegisterPluginsPropagatesFailure(t *testing.T) {
	t.Parallel()

	plugins, err := registerPlugins(sdk.Deps{DatabaseURL: "://not-a-url"})

	if err == nil {
		t.Fatal("registerPlugins() error = nil, want a parse error")
	}
	if plugins != nil {
		t.Errorf("registerPlugins() = %v, want nil on failure", plugins)
	}
}

var errRegistration = errors.New("registration exploded")

func TestRunReportsRegistrationFailure(t *testing.T) {
	t.Parallel()

	err := run(t.Context(), testGetenv(map[string]string{
		"ALPHONE_DATABASE_URL": testDatabaseURL(t),
	}), io.Discard, func(_ sdk.Deps) ([]sdk.Plugin, error) {
		return nil, errRegistration
	})

	if !errors.Is(err, errRegistration) {
		t.Fatalf("run() error = %v, want %v in its chain", err, errRegistration)
	}
}

func TestRunRejectsMalformedDatabaseURL(t *testing.T) {
	t.Parallel()

	err := run(t.Context(), testGetenv(map[string]string{
		"ALPHONE_DATABASE_URL": "://not-a-url",
	}), io.Discard, registerPlugins)

	if err == nil {
		t.Fatal("run() error = nil, want a parse error")
	}
}

func TestRunReportsMigrationFailure(t *testing.T) {
	t.Parallel()

	err := run(t.Context(), testGetenv(map[string]string{
		"ALPHONE_DATABASE_URL": "postgres://postgres:alphone@localhost:9/postgres?sslmode=disable&connect_timeout=1",
	}), io.Discard, registerPlugins)

	if err == nil {
		t.Fatal("run() error = nil, want a migration error")
	}
}

func TestRunReportsBindFailure(t *testing.T) {
	t.Parallel()

	listener, err := net.Listen("tcp", "localhost:0")
	if err != nil {
		t.Fatalf("occupying a port: %v", err)
	}
	defer func() { _ = listener.Close() }()

	err = run(t.Context(), testGetenv(map[string]string{
		"ALPHONE_DATABASE_URL": testDatabaseURL(t),
		"ALPHONE_ADDR":         listener.Addr().String(),
	}), io.Discard, registerPlugins)

	if err == nil || !strings.Contains(err.Error(), "http server") {
		t.Fatalf("run() error = %v, want a bind failure", err)
	}
}

func doAuthed(
	t *testing.T,
	ctx context.Context,
	session *http.Cookie,
	method, url, body string,
) *http.Response {
	t.Helper()
	var reader io.Reader
	if body != "" {
		reader = strings.NewReader(body)
	}
	request, err := http.NewRequestWithContext(ctx, method, url, reader)
	if err != nil {
		t.Fatalf("building %s %s: %v", method, url, err)
	}
	request.Header.Set("Content-Type", "application/json")
	request.AddCookie(session)
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatalf("%s %s: %v", method, url, err)
	}
	return response
}

func TestRunServesAPI(t *testing.T) {
	t.Parallel()

	addr := freeAddr(t)
	databaseURL := testDatabaseURL(t)
	webDir := t.TempDir()
	if err := os.WriteFile(webDir+"/index.html", []byte("<!doctype html><title>AlphOne</title>"), 0o644); err != nil {
		t.Fatalf("writing index.html: %v", err)
	}
	ctx, cancel := context.WithCancel(t.Context())
	runErr := make(chan error, 1)
	go func() {
		runErr <- run(ctx, testGetenv(map[string]string{
			"ALPHONE_DATABASE_URL":          databaseURL,
			"ALPHONE_ADDR":                  addr,
			"ALPHONE_WEB_DIR":               webDir,
			"ALPHONE_WHATSAPP_VERIFY_TOKEN": "e2e-secret",
			"ALPHONE_WHATSAPP_APP_SECRET":   "e2e-app-secret",
		}), io.Discard, registerPlugins)
	}()

	baseURL := "http://" + addr
	waitForServer(t, baseURL+"/api/contacts/"+uuid.Must(uuid.NewV7()).String())

	spa, err := http.Get(baseURL + "/")
	if err != nil {
		t.Fatalf("GET /: %v", err)
	}
	spaBody, _ := io.ReadAll(spa.Body)
	_ = spa.Body.Close()
	if spa.StatusCode != http.StatusOK || !strings.Contains(string(spaBody), "AlphOne") {
		t.Fatalf("GET / = %d %q, want the served SPA index.html", spa.StatusCode, spaBody)
	}

	unauthorized, err := http.Post(
		baseURL+"/api/contacts", "application/json", strings.NewReader(`{"name":"María Pérez"}`),
	)
	if err != nil {
		t.Fatalf("POST /api/contacts without a session: %v", err)
	}
	defer func() { _ = unauthorized.Body.Close() }()
	if unauthorized.StatusCode != http.StatusUnauthorized {
		t.Fatalf("unauthenticated POST status = %d, want %d", unauthorized.StatusCode, http.StatusUnauthorized)
	}

	pool, err := pgxpool.New(t.Context(), databaseURL)
	if err != nil {
		t.Fatalf("connecting pool: %v", err)
	}
	t.Cleanup(pool.Close)
	admin, err := gouncer.NewUser("admin@example.com", "Admin", "correct horse battery")
	if err != nil {
		t.Fatalf("gouncer.NewUser() error = %v, want nil", err)
	}
	if err := postgres.NewUserStore(pool).CreateUser(t.Context(), admin); err != nil {
		t.Fatalf("CreateUser() error = %v, want nil", err)
	}

	login, err := http.Post(
		baseURL+"/api/auth/login",
		"application/json",
		strings.NewReader(`{"email":"admin@example.com","password":"correct horse battery"}`),
	)
	if err != nil {
		t.Fatalf("POST /api/auth/login: %v", err)
	}
	defer func() { _ = login.Body.Close() }()
	if login.StatusCode != http.StatusOK {
		t.Fatalf("login status = %d, want %d", login.StatusCode, http.StatusOK)
	}
	var session *http.Cookie
	for _, cookie := range login.Cookies() {
		if cookie.Name == "__Host-alphone_session" {
			session = cookie
		}
	}
	if session == nil {
		t.Fatal("login response carries no alphone_session cookie")
	}

	createContact := doAuthed(t, ctx, session, http.MethodPost, baseURL+"/api/contacts",
		`{"name":"María Pérez"}`)
	_ = createContact.Body.Close()
	if createContact.StatusCode != http.StatusCreated {
		t.Fatalf("POST status = %d, want %d", createContact.StatusCode, http.StatusCreated)
	}

	createUser := doAuthed(t, ctx, session, http.MethodPost, baseURL+"/api/users",
		`{"email":"grace@example.com","name":"Grace Hopper","password":"correct horse battery"}`)
	if createUser.StatusCode != http.StatusCreated {
		t.Fatalf("POST /api/users status = %d, want %d", createUser.StatusCode, http.StatusCreated)
	}
	var created struct {
		ID uuid.UUID `json:"id"`
	}
	if err := json.NewDecoder(createUser.Body).Decode(&created); err != nil {
		t.Fatalf("decoding created user: %v", err)
	}
	_ = createUser.Body.Close()

	listUsers := doAuthed(t, ctx, session, http.MethodGet, baseURL+"/api/users", "")
	body, _ := io.ReadAll(listUsers.Body)
	_ = listUsers.Body.Close()
	if listUsers.StatusCode != http.StatusOK || !strings.Contains(string(body), "grace@example.com") {
		t.Fatalf("GET /api/users = %d %q, want %d listing the new user", listUsers.StatusCode, body, http.StatusOK)
	}

	disableUser := doAuthed(t, ctx, session, http.MethodPatch,
		baseURL+"/api/users/"+created.ID.String(), `{"disabled":true}`)
	_ = disableUser.Body.Close()
	if disableUser.StatusCode != http.StatusNoContent {
		t.Fatalf("PATCH /api/users status = %d, want %d", disableUser.StatusCode, http.StatusNoContent)
	}

	verification, err := http.Get(
		baseURL + "/api/plugins/whatsapp/webhook?hub.mode=subscribe&hub.verify_token=e2e-secret&hub.challenge=42",
	)
	if err != nil {
		t.Fatalf("GET webhook verification: %v", err)
	}
	defer func() { _ = verification.Body.Close() }()
	challenge, err := io.ReadAll(verification.Body)
	if err != nil {
		t.Fatalf("reading challenge: %v", err)
	}
	if verification.StatusCode != http.StatusOK || string(challenge) != "42" {
		t.Fatalf("webhook verification = %d %q, want %d %q", verification.StatusCode, challenge, http.StatusOK, "42")
	}

	event := []byte(`{"entry":[{"changes":[{"value":{"contacts":[{"wa_id":"184467235",` +
		`"profile":{"name":"María Pérez"}}],"messages":[{"from":"184467235",` +
		`"id":"wamid.e2e","timestamp":"1751791000","type":"text","text":{"body":"hola"}}]}}]}]}`)
	mac := hmac.New(sha256.New, []byte("e2e-app-secret"))
	mac.Write(event)
	eventRequest, err := http.NewRequestWithContext(
		ctx,
		http.MethodPost,
		baseURL+"/api/plugins/whatsapp/webhook",
		bytes.NewReader(event),
	)
	if err != nil {
		t.Fatalf("building event request: %v", err)
	}
	eventRequest.Header.Set("X-Hub-Signature-256", "sha256="+hex.EncodeToString(mac.Sum(nil)))
	eventResponse, err := http.DefaultClient.Do(eventRequest)
	if err != nil {
		t.Fatalf("POST webhook event: %v", err)
	}
	defer func() { _ = eventResponse.Body.Close() }()
	if eventResponse.StatusCode != http.StatusOK {
		t.Fatalf("webhook event status = %d, want %d", eventResponse.StatusCode, http.StatusOK)
	}

	cancel()
	select {
	case err := <-runErr:
		if err != nil {
			t.Fatalf("run() error = %v, want nil after graceful shutdown", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("run() did not return after context cancellation")
	}
}
