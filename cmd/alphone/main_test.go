// SPDX-License-Identifier: AGPL-3.0-or-later

package main

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"net"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/peterldowns/pgtestdb"

	"github.com/gopherium/alphone/internal/plugin"
	"github.com/gopherium/alphone/internal/testdb"
)

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

func failingPlugins(_ *pgxpool.Pool, _ func(string) string) []plugin.Plugin {
	return []plugin.Plugin{failingPlugin{}}
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
			defer response.Body.Close()
			if response.StatusCode == http.StatusNotFound {
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
	defer listener.Close()

	err = run(t.Context(), testGetenv(map[string]string{
		"ALPHONE_DATABASE_URL": testDatabaseURL(t),
		"ALPHONE_ADDR":         listener.Addr().String(),
	}), io.Discard, registerPlugins)

	if err == nil || !strings.Contains(err.Error(), "http server") {
		t.Fatalf("run() error = %v, want a bind failure", err)
	}
}

func TestRunServesAPI(t *testing.T) {
	t.Parallel()

	addr := freeAddr(t)
	ctx, cancel := context.WithCancel(t.Context())
	runErr := make(chan error, 1)
	go func() {
		runErr <- run(ctx, testGetenv(map[string]string{
			"ALPHONE_DATABASE_URL":          testDatabaseURL(t),
			"ALPHONE_ADDR":                  addr,
			"ALPHONE_WHATSAPP_VERIFY_TOKEN": "e2e-secret",
			"ALPHONE_WHATSAPP_APP_SECRET":   "e2e-app-secret",
		}), io.Discard, registerPlugins)
	}()

	baseURL := "http://" + addr
	waitForServer(t, baseURL+"/api/contacts/"+uuid.Must(uuid.NewV7()).String())

	response, err := http.Post(baseURL+"/api/contacts", "application/json", strings.NewReader(`{"name":"María Pérez"}`))
	if err != nil {
		t.Fatalf("POST /api/contacts: %v", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusCreated {
		t.Fatalf("POST status = %d, want %d", response.StatusCode, http.StatusCreated)
	}

	verification, err := http.Get(baseURL + "/api/plugins/whatsapp/webhook?hub.mode=subscribe&hub.verify_token=e2e-secret&hub.challenge=42")
	if err != nil {
		t.Fatalf("GET webhook verification: %v", err)
	}
	defer verification.Body.Close()
	challenge, err := io.ReadAll(verification.Body)
	if err != nil {
		t.Fatalf("reading challenge: %v", err)
	}
	if verification.StatusCode != http.StatusOK || string(challenge) != "42" {
		t.Fatalf("webhook verification = %d %q, want %d %q", verification.StatusCode, challenge, http.StatusOK, "42")
	}

	event := []byte(`{"entry":[{"changes":[{"value":{"contacts":[{"wa_id":"184467235","profile":{"name":"María Pérez"}}],"messages":[{"from":"184467235","id":"wamid.e2e","timestamp":"1751791000","type":"text","text":{"body":"hola"}}]}}]}]}`)
	mac := hmac.New(sha256.New, []byte("e2e-app-secret"))
	mac.Write(event)
	eventRequest, err := http.NewRequestWithContext(ctx, http.MethodPost, baseURL+"/api/plugins/whatsapp/webhook", bytes.NewReader(event))
	if err != nil {
		t.Fatalf("building event request: %v", err)
	}
	eventRequest.Header.Set("X-Hub-Signature-256", "sha256="+hex.EncodeToString(mac.Sum(nil)))
	eventResponse, err := http.DefaultClient.Do(eventRequest)
	if err != nil {
		t.Fatalf("POST webhook event: %v", err)
	}
	defer eventResponse.Body.Close()
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
