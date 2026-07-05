// SPDX-License-Identifier: AGPL-3.0-or-later

package main

import (
	"context"
	"io"
	"net"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/peterldowns/pgtestdb"
	"github.com/peterldowns/pgtestdb/migrators/goosemigrator"

	"github.com/gopherium/alphone/internal/postgres"
)

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
	cfg := pgtestdb.Custom(t, pgtestdb.Config{
		DriverName: "pgx",
		User:       "postgres",
		Password:   "alphone",
		Host:       "localhost",
		Port:       "5433",
		Database:   "postgres",
		Options:    "sslmode=disable",
	}, goosemigrator.New("migrations", goosemigrator.WithFS(postgres.Migrations)))
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

	err := run(t.Context(), testGetenv(nil), io.Discard)

	if err == nil {
		t.Fatal("run() error = nil, want a configuration error")
	}
}

func TestRunRejectsMalformedDatabaseURL(t *testing.T) {
	t.Parallel()

	err := run(t.Context(), testGetenv(map[string]string{
		"ALPHONE_DATABASE_URL": "://not-a-url",
	}), io.Discard)

	if err == nil {
		t.Fatal("run() error = nil, want a parse error")
	}
}

func TestRunReportsMigrationFailure(t *testing.T) {
	t.Parallel()

	err := run(t.Context(), testGetenv(map[string]string{
		"ALPHONE_DATABASE_URL": "postgres://postgres:alphone@localhost:9/postgres?sslmode=disable&connect_timeout=1",
	}), io.Discard)

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
	}), io.Discard)

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
			"ALPHONE_DATABASE_URL": testDatabaseURL(t),
			"ALPHONE_ADDR":         addr,
		}), io.Discard)
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
