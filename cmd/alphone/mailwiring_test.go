// SPDX-License-Identifier: Elastic-2.0

package main

import (
	"context"
	"encoding/json"
	"io"
	"mime/quotedprintable"
	"net/http"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	smtpmock "github.com/mocktools/go-smtp-mock/v2"

	"github.com/gopherium/gouncer"
	authkitpg "github.com/gopherium/gouncer/authkit/postgres"

	"github.com/gopherium/alphone/internal/role"
)

// mailRelay starts a mock relay that survives the reset go-mail sends after delivering.
func mailRelay(t *testing.T) *smtpmock.Server {
	t.Helper()
	server := smtpmock.New(smtpmock.ConfigurationAttr{MultipleMessageReceiving: true})
	if err := server.Start(); err != nil {
		t.Fatalf("starting the mock relay: %v", err)
	}
	t.Cleanup(func() { _ = server.Stop() })
	return server
}

// stopRun cancels the server and waits for run to release its pool before the test database is dropped.
func stopRun(t *testing.T, cancel context.CancelFunc, runErr <-chan error) {
	t.Helper()
	cancel()
	select {
	case err := <-runErr:
		if err != nil {
			t.Errorf("run() error = %v, want nil after graceful shutdown", err)
		}
	case <-time.After(5 * time.Second):
		t.Error("run() did not return after context cancellation")
	}
}

// seedAdmin stores an enabled admin account able to invite.
func seedAdmin(t *testing.T, databaseURL string) {
	t.Helper()
	pool, err := pgxpool.New(t.Context(), databaseURL)
	if err != nil {
		t.Fatalf("connecting pool: %v", err)
	}
	t.Cleanup(pool.Close)
	admin, err := gouncer.NewUser("admin@example.com", "Admin", "correct horse battery")
	if err != nil {
		t.Fatalf("gouncer.NewUser() error = %v, want nil", err)
	}
	admin.Role = role.Admin.String()
	if err := authkitpg.NewUserStore(pool).CreateUser(t.Context(), admin); err != nil {
		t.Fatalf("CreateUser() error = %v, want nil", err)
	}
}

// loginSession logs the seeded admin in and answers the session cookie.
func loginSession(t *testing.T, baseURL string) *http.Cookie {
	t.Helper()
	login, err := http.Post(baseURL+"/api/graphql", "application/json", strings.NewReader(
		`{"query":"mutation { login(email: \"admin@example.com\",`+
			` password: \"correct horse battery\") { me { id } } }"}`))
	if err != nil {
		t.Fatalf("graph login: %v", err)
	}
	defer func() { _ = login.Body.Close() }()
	for _, cookie := range login.Cookies() {
		if cookie.Name == "__Host-alphone_session" {
			return cookie
		}
	}
	t.Fatal("login response carries no session cookie")
	return nil
}

// postGraphAnonymously posts an anonymous graph body and answers the raw response body.
func postGraphAnonymously(t *testing.T, baseURL, body string) string {
	t.Helper()
	response, err := http.Post(baseURL+"/api/graphql", "application/json", strings.NewReader(body))
	if err != nil {
		t.Fatalf("posting the graph body: %v", err)
	}
	answer, err := io.ReadAll(response.Body)
	_ = response.Body.Close()
	if err != nil {
		t.Fatalf("reading the graph answer: %v", err)
	}
	return string(answer)
}

// mailBody decodes the quoted-printable body of a delivered message.
func mailBody(t *testing.T, payload string) string {
	t.Helper()
	_, encoded, found := strings.Cut(payload, "\r\n\r\n")
	if !found {
		t.Fatalf("mail = %q, want headers followed by a body", payload)
	}
	decoded, err := io.ReadAll(quotedprintable.NewReader(strings.NewReader(encoded)))
	if err != nil {
		t.Fatalf("decoding the mail body: %v", err)
	}
	return string(decoded)
}

// tokenFromMail pulls the activation token out of the one mail the relay received.
func tokenFromMail(t *testing.T, relay *smtpmock.Server, base string) string {
	t.Helper()
	messages, err := relay.WaitForMessages(1, 5*time.Second)
	if err != nil {
		t.Fatalf("waiting for the invitation mail: %v", err)
	}
	body := mailBody(t, messages[0].MsgRequest())
	_, after, found := strings.Cut(body, base+"/activate?token=")
	if !found {
		t.Fatalf("mail body = %q, want an activation link under %s", body, base)
	}
	token, _, _ := strings.Cut(after, "\n")
	return strings.TrimSpace(token)
}

func TestRunAnswersTheActivationLinkWithoutARelay(t *testing.T) {
	t.Parallel()

	if testing.Short() {
		t.Skip("skipping database test in short mode")
	}
	addr := freeAddr(t)
	databaseURL := testDatabaseURL(t)
	ctx, cancel := context.WithCancel(t.Context())
	runErr := make(chan error, 1)
	go func() {
		runErr <- run(ctx, testGetenv(map[string]string{
			"ALPHONE_DATABASE_URL": databaseURL,
			"ALPHONE_ADDR":         addr,
		}), io.Discard, registerPlugins)
	}()
	t.Cleanup(func() { stopRun(t, cancel, runErr) })

	baseURL := "http://" + addr
	waitForServer(t, baseURL)
	seedAdmin(t, databaseURL)
	session := loginSession(t, baseURL)

	invited := postGraphAuthed(t, ctx, session, baseURL,
		`{"query":"mutation { invite(email: \"grace@example.com\", name: \"Grace Hopper\")`+
			` { delivered activationLink } }"}`)

	if !strings.Contains(invited, `"delivered":false`) {
		t.Fatalf("invite = %q, want it undelivered without a relay", invited)
	}
	if !strings.Contains(invited, `"activationLink":"/activate?token=`) {
		t.Fatalf("invite = %q, want the link answered for hand delivery", invited)
	}
}

func TestRunMailsAnInvitationThatActivates(t *testing.T) {
	t.Parallel()

	if testing.Short() {
		t.Skip("skipping database test in short mode")
	}
	relay := mailRelay(t)
	addr := freeAddr(t)
	databaseURL := testDatabaseURL(t)
	const publicURL = "https://crm.example.com"
	ctx, cancel := context.WithCancel(t.Context())
	runErr := make(chan error, 1)
	go func() {
		runErr <- run(ctx, testGetenv(map[string]string{
			"ALPHONE_DATABASE_URL": databaseURL,
			"ALPHONE_ADDR":         addr,
			"ALPHONE_SMTP_HOST":    "127.0.0.1",
			"ALPHONE_SMTP_PORT":    strconv.Itoa(relay.PortNumber()),
			"ALPHONE_SMTP_FROM":    "crm@example.com",
			"ALPHONE_SMTP_TLS":     "none",
			"ALPHONE_PUBLIC_URL":   publicURL,
		}), io.Discard, registerPlugins)
	}()
	t.Cleanup(func() { stopRun(t, cancel, runErr) })

	baseURL := "http://" + addr
	waitForServer(t, baseURL)
	seedAdmin(t, databaseURL)
	session := loginSession(t, baseURL)

	invited := postGraphAuthed(t, ctx, session, baseURL,
		`{"query":"mutation { invite(email: \"grace@example.com\", name: \"Grace Hopper\")`+
			` { delivered activationLink } }"}`)
	if !strings.Contains(invited, `"delivered":true`) {
		t.Fatalf("invite = %q, want it delivered by the relay", invited)
	}

	token := tokenFromMail(t, relay, publicURL)

	activated := postGraphAnonymously(t, baseURL,
		`{"query":"mutation { acceptInvite(token: \"`+token+
			`\", password: \"correct horse battery\") { me { id email } } }"}`)
	var answer struct {
		Data struct {
			AcceptInvite struct {
				Me struct {
					ID    uuid.UUID `json:"id"`
					Email string    `json:"email"`
				} `json:"me"`
			} `json:"acceptInvite"`
		} `json:"data"`
	}
	if err := json.Unmarshal([]byte(activated), &answer); err != nil {
		t.Fatalf("decoding the activation from %q: %v", activated, err)
	}
	if answer.Data.AcceptInvite.Me.Email != "grace@example.com" {
		t.Fatalf("acceptInvite = %q, want Grace Hopper's activated identity", activated)
	}
	if answer.Data.AcceptInvite.Me.ID == uuid.Nil {
		t.Fatalf("acceptInvite = %q, want the activated account's id", activated)
	}

	listed := postGraphAuthed(t, ctx, session, baseURL, `{"query":"{ users { email confirmed } }"}`)
	if !strings.Contains(listed, `"email":"grace@example.com","confirmed":true`) {
		t.Fatalf("users = %q, want the activated account listed confirmed", listed)
	}
}
