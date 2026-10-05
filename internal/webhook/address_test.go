// SPDX-License-Identifier: Elastic-2.0

package webhook_test

import (
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"reflect"
	"strconv"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gopherium/alphone/internal/webhook"
)

// prefixes parses the ranges a test expects.
func prefixes(held ...string) []netip.Prefix {
	parsed := make([]netip.Prefix, 0, len(held))
	for _, raw := range held {
		parsed = append(parsed, netip.MustParsePrefix(raw))
	}
	return parsed
}

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

func TestParseAllowListReadsRangesAndHostNamesWithAPort(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		raw  string
		want webhook.AllowList
	}{
		"blank entries only": {raw: " , ", want: webhook.AllowList{}},
		"one range":          {raw: "127.0.0.1/32", want: webhook.AllowList{Ranges: prefixes("127.0.0.1/32")}},
		"a range masked":     {raw: "10.1.2.3/8", want: webhook.AllowList{Ranges: prefixes("10.0.0.0/8")}},
		"an IPv6 range":      {raw: "fc00::/7", want: webhook.AllowList{Ranges: prefixes("fc00::/7")}},
		"one host":           {raw: "n8n:5678", want: webhook.AllowList{Hosts: []string{"n8n:5678"}}},
		"a host in capitals": {raw: "N8N.Example.COM:443", want: webhook.AllowList{Hosts: []string{"n8n.example.com:443"}}},
		"a padded port":      {raw: "n8n:05678", want: webhook.AllowList{Hosts: []string{"n8n:5678"}}},
		"an IPv6 host":       {raw: "[::1]:5678", want: webhook.AllowList{Hosts: []string{"[::1]:5678"}}},
		"both kinds trimmed": {
			raw:  " 127.0.0.1/32 , localhost:5678 ,n8n:5678",
			want: webhook.AllowList{Ranges: prefixes("127.0.0.1/32"), Hosts: []string{"localhost:5678", "n8n:5678"}},
		},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			got, err := webhook.ParseAllowList(tc.raw)

			if err != nil {
				t.Fatalf("ParseAllowList(%q) error = %v, want nil", tc.raw, err)
			}
			if !reflect.DeepEqual(got, tc.want) {
				t.Errorf("ParseAllowList(%q) = %+v, want %+v", tc.raw, got, tc.want)
			}
		})
	}
}

func TestParseAllowListRefusesAnEntryItCannotRead(t *testing.T) {
	t.Parallel()

	for _, raw := range []string{
		"n8n", "n8n:", ":5678", "n8n:http", "n8n:0", "n8n:70000", "10.0.0.1", "10.0.0.0/33",
		"http://n8n:5678", "n8n:5678/hook", "127.0.0.1/32,nonsense",
	} {
		_, err := webhook.ParseAllowList(raw)

		want := `must list CIDR ranges such as 127.0.0.1/32 or host names with a port such as n8n:5678, got "` + raw + `"`
		if err == nil || err.Error() != want {
			t.Errorf("ParseAllowList(%q) error = %v, want %q", raw, err, want)
		}
	}
}

func TestTheClientReachesAHostEntryAnOperatorAllowed(t *testing.T) {
	t.Parallel()

	server, hits := countingServer(t)
	port := server.Listener.Addr().(*net.TCPAddr).Port
	guard := webhook.AddressGuard{Allowed: webhook.AllowList{Hosts: []string{"localhost:" + strconv.Itoa(port)}}}

	response, err := guard.Client(time.Second).Get("http://LocalHost:" + strconv.Itoa(port) + "/hook")

	if err != nil {
		t.Fatalf("Get() error = %v, want the allowed host reached", err)
	}
	_ = response.Body.Close()
	if got := hits.Load(); got != 1 {
		t.Errorf("server saw %d requests, want 1", got)
	}
}

func TestAHostEntryOpensItsNameAndPortAlone(t *testing.T) {
	t.Parallel()

	server, hits := countingServer(t)
	port := strconv.Itoa(server.Listener.Addr().(*net.TCPAddr).Port)
	guard := webhook.AddressGuard{Allowed: webhook.AllowList{Hosts: []string{"localhost:1"}}}

	for _, target := range []string{"http://localhost:" + port + "/hook", "http://127.0.0.1:" + port + "/hook"} {
		response, err := guard.Client(time.Second).Get(target)

		if response != nil {
			_ = response.Body.Close()
		}
		if !errors.Is(err, webhook.ErrAddressRefused) {
			t.Errorf("Get(%q) error = %v, want %v", target, err, webhook.ErrAddressRefused)
		}
	}
	if got := hits.Load(); got != 0 {
		t.Errorf("server saw %d requests, want none", got)
	}
}

func TestTheClientFollowsNoRedirect(t *testing.T) {
	t.Parallel()

	second, reached := countingServer(t)
	first := httptest.NewServer(http.RedirectHandler(second.URL, http.StatusFound))
	t.Cleanup(first.Close)
	guard := webhook.AddressGuard{Allowed: webhook.AllowList{Ranges: prefixes("127.0.0.1/32")}}

	response, err := guard.Client(time.Second).Get(first.URL)

	if err != nil {
		t.Fatalf("Get() error = %v, want the redirect answered as it came", err)
	}
	_ = response.Body.Close()
	if response.StatusCode != http.StatusFound {
		t.Errorf("status = %d, want %d", response.StatusCode, http.StatusFound)
	}
	if got := reached.Load(); got != 0 {
		t.Errorf("the redirect target saw %d requests, want none", got)
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
