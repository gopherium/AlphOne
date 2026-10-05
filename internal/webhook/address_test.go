// SPDX-License-Identifier: Elastic-2.0

package webhook_test

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gopherium/alphone/internal/webhook"
)

// countingServer starts a server answering 204 and counting every request it sees.
func countingServer(t *testing.T) (*httptest.Server, *atomic.Int32) {
	t.Helper()
	var hits atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		hits.Add(1)
		w.WriteHeader(http.StatusNoContent)
	}))
	t.Cleanup(server.Close)
	return server, &hits
}

func TestTheClientRefusesALoopbackServerBeforeItConnects(t *testing.T) {
	t.Parallel()

	server, hits := countingServer(t)

	response, err := (webhook.AddressGuard{}).Client(time.Second).Get(server.URL)

	if response != nil {
		_ = response.Body.Close()
	}
	if !errors.Is(err, webhook.ErrAddressRefused) {
		t.Errorf("Get() error = %v, want %v", err, webhook.ErrAddressRefused)
	}
	if got := hits.Load(); got != 0 {
		t.Errorf("server saw %d requests, want none", got)
	}
}

func TestTheClientReachesALoopbackServerAnOperatorAllowed(t *testing.T) {
	t.Parallel()

	server, hits := countingServer(t)
	guard := webhook.AddressGuard{
		Allowed: webhook.AllowList{Ranges: []netip.Prefix{netip.MustParsePrefix("127.0.0.1/32")}},
	}

	response, err := guard.Client(time.Second).Get(server.URL)

	if err != nil {
		t.Fatalf("Get() error = %v, want nil", err)
	}
	_ = response.Body.Close()
	if got := hits.Load(); got != 1 {
		t.Errorf("server saw %d requests, want 1", got)
	}
}
