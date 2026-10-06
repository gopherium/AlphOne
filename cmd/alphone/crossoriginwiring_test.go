// SPDX-License-Identifier: Elastic-2.0

package main

import (
	"bytes"
	"context"
	"net/http"
	"strings"
	"sync"
	"testing"
)

// lockedLog is a log sink the server writes while the test reads it.
type lockedLog struct {
	mu      sync.Mutex
	written bytes.Buffer
}

// Write appends p to the log.
func (l *lockedLog) Write(p []byte) (int, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.written.Write(p)
}

// String returns the log written so far.
func (l *lockedLog) String() string {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.written.String()
}

// olderBrowserWrite posts an older browser's graph write from origin, adding the forwarded headers when named.
func olderBrowserWrite(t *testing.T, ctx context.Context, baseURL, origin, forwardedHost string) int {
	t.Helper()
	request, err := http.NewRequestWithContext(
		ctx, http.MethodPost, baseURL+"/api/graphql", strings.NewReader(versionProbe))
	if err != nil {
		t.Fatalf("building the write: %v", err)
	}
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Origin", origin)
	if forwardedHost != "" {
		request.Header.Set("X-Forwarded-Host", forwardedHost)
		request.Header.Set("X-Forwarded-Proto", "https")
	}
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatalf("posting the write: %v", err)
	}
	_ = response.Body.Close()
	return response.StatusCode
}

func TestRunJudgesAnOlderBrowserWriteOnTheHostHeaderAndLogsARefusal(t *testing.T) {
	t.Parallel()

	if testing.Short() {
		t.Skip("skipping database test in short mode")
	}
	addr := freeAddr(t)
	databaseURL := testDatabaseURL(t)
	logged := &lockedLog{}
	ctx, cancel := context.WithCancel(t.Context())
	runErr := make(chan error, 1)
	go func() {
		runErr <- run(ctx, testGetenv(map[string]string{
			"ALPHONE_DATABASE_URL":    databaseURL,
			"ALPHONE_ADDR":            addr,
			"ALPHONE_TRUSTED_PROXIES": "127.0.0.0/8,::1/128",
		}), logged, registerPlugins)
	}()
	t.Cleanup(func() { stopRun(t, cancel, runErr) })
	baseURL := "http://" + addr
	waitForServer(t, baseURL)

	if status := olderBrowserWrite(t, ctx, baseURL, baseURL, ""); status != http.StatusOK {
		t.Errorf("a write from the server's own page = %d, want %d", status, http.StatusOK)
	}
	status := olderBrowserWrite(t, ctx, baseURL, "https://crm.example.com", "crm.example.com")
	if status != http.StatusForbidden {
		t.Errorf("a write a trusted proxy relays for crm.example.com = %d, want %d", status, http.StatusForbidden)
	}
	want := `msg="write refused" reason=origin method=POST path=/api/graphql host=` + addr
	if log := logged.String(); !strings.Contains(log, want) {
		t.Errorf("log = %q, want it to hold %q", log, want)
	}
}
