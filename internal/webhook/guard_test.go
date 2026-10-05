// SPDX-License-Identifier: Elastic-2.0

package webhook_test

import (
	"net/http"
	"net/http/httptest"
	"net/netip"
	"net/url"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/google/uuid"

	"github.com/gopherium/alphone/internal/event"
	"github.com/gopherium/alphone/internal/webhook"
)

// countingSubscriber starts a subscriber answering 200 and counting every post it sees.
func countingSubscriber(t *testing.T) (*httptest.Server, *atomic.Int32) {
	t.Helper()
	var posts atomic.Int32
	subscriber := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		posts.Add(1)
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(subscriber.Close)
	return subscriber, &posts
}

// portOf returns the port a test server listens on.
func portOf(t *testing.T, server *httptest.Server) string {
	t.Helper()
	parsed, err := url.Parse(server.URL)
	if err != nil {
		t.Fatalf("reading the server address %q: %v", server.URL, err)
	}
	return parsed.Port()
}

// deliverOnce queues one delivery to target, sweeps it through a worker under guard, and returns its settlement.
func deliverOnce(t *testing.T, target string, guard webhook.AddressGuard) (settlement, string) {
	t.Helper()
	queue := &fakeWorkerQueue{pending: []webhook.ClaimedDelivery{claimed(t, target, "whsec_a", 1)}}
	worker, logged := newGuardedWorker(queue, guard)

	worker.Sweep(t.Context())

	settled := queue.settlements()
	if len(settled) != 1 {
		t.Fatalf("settled %d deliveries, want 1", len(settled))
	}
	return settled[0], logged.String()
}

// wantRefused fails the test unless the settlement and the log record a refused delivery to host.
func wantRefused(t *testing.T, settled settlement, logged, host string) {
	t.Helper()
	if settled.Status != webhook.StatusPending {
		t.Errorf("status = %q, want a refused delivery left pending as a failed attempt", settled.Status)
	}
	if !strings.Contains(settled.LastError, webhook.ErrAddressRefused.Error()) {
		t.Errorf("last_error = %q, want the refusal recorded", settled.LastError)
	}
	if !strings.Contains(logged, "level=WARN") || !strings.Contains(logged, "host="+host) {
		t.Errorf("log = %q, want a warning naming the host %s", logged, host)
	}
}

func TestWorkerRefusesALoopbackSubscriberWithNothingAllowed(t *testing.T) {
	t.Parallel()

	subscriber, posts := countingSubscriber(t)

	settled, logged := deliverOnce(t, subscriber.URL+"/hook", webhook.AddressGuard{})

	wantRefused(t, settled, logged, strings.TrimPrefix(subscriber.URL, "http://"))
	if got := posts.Load(); got != 0 {
		t.Errorf("subscriber saw %d posts, want the delivery refused before it connects", got)
	}
}

func TestWorkerRefusesEveryInternalRange(t *testing.T) {
	t.Parallel()

	for name, host := range map[string]string{
		"cloud metadata":          "169.254.169.254",
		"container credentials":   "169.254.170.2",
		"a private ten":           "10.0.0.1",
		"a private ten for pods":  "10.42.0.10",
		"a private seventeen":     "172.16.0.1",
		"a container bridge":      "172.17.0.2",
		"a private one ninety":    "192.168.1.1",
		"shared address space":    "100.64.0.1",
		"this network":            "0.0.0.0",
		"loopback six":            "[::1]",
		"metadata six":            "[fd00:ec2::254]",
		"a container network six": "[fd12:3456::1]",
		"link local six":          "[fe80::1]",
		"translated six":          "[64:ff9b::7f00:1]",
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			settled, logged := deliverOnce(t, "http://"+host+"/hook", webhook.AddressGuard{})

			wantRefused(t, settled, logged, host)
		})
	}
}

func TestWorkerRefusesTheMetadataAddressEvenWhenItsRangeIsAllowed(t *testing.T) {
	t.Parallel()

	guard := webhook.AddressGuard{
		Allowed: webhook.AllowList{Ranges: []netip.Prefix{netip.MustParsePrefix("169.254.0.0/16")}},
	}

	settled, logged := deliverOnce(t, "http://169.254.169.254/latest", guard)

	wantRefused(t, settled, logged, "169.254.169.254")
}

func TestWorkerRefusesLoopbackWrittenAsAnIPv4InIPv6Address(t *testing.T) {
	t.Parallel()

	subscriber, posts := countingSubscriber(t)
	port := portOf(t, subscriber)

	settled, logged := deliverOnce(t, "http://[::ffff:127.0.0.1]:"+port+"/hook", webhook.AddressGuard{})

	wantRefused(t, settled, logged, "[::ffff:127.0.0.1]:"+port)
	if !strings.Contains(settled.LastError, "tcp4 127.0.0.1:"+port) {
		t.Errorf("last_error = %q, want the guard to have seen the plain tcp4 loopback address", settled.LastError)
	}
	if got := posts.Load(); got != 0 {
		t.Errorf("subscriber saw %d posts, want none", got)
	}
}

func TestWorkerChecksTheAddressANameResolvesToWhenItDials(t *testing.T) {
	t.Parallel()

	subscriber, posts := countingSubscriber(t)
	target := "http://rebound.example.com:" + portOf(t, subscriber) + "/hook"
	if _, err := webhook.NewSubscription(uuid.Must(uuid.NewV7()), target, []event.Name{event.TaskCreated}); err != nil {
		t.Fatalf("NewSubscription() error = %v, want the name accepted when the webhook is created", err)
	}
	rebound := fakeResolver(map[string]netip.Addr{"rebound.example.com": netip.MustParseAddr("127.0.0.1")})

	settled, logged := deliverOnce(t, target, webhook.AddressGuard{Resolver: rebound})

	wantRefused(t, settled, logged, "rebound.example.com:"+portOf(t, subscriber))
	if got := posts.Load(); got != 0 {
		t.Errorf("subscriber saw %d posts, want the resolved loopback address refused", got)
	}
}

func TestWorkerDeliversToANameResolvingInsideAnAllowedRange(t *testing.T) {
	t.Parallel()

	subscriber, posts := countingSubscriber(t)
	target := "http://rebound.example.com:" + portOf(t, subscriber) + "/hook"
	guard := loopback
	guard.Resolver = fakeResolver(map[string]netip.Addr{"rebound.example.com": netip.MustParseAddr("127.0.0.1")})

	settled, _ := deliverOnce(t, target, guard)

	if settled.Status != webhook.StatusDelivered {
		t.Errorf("status = %q, want delivered: %s", settled.Status, settled.LastError)
	}
	if got := posts.Load(); got != 1 {
		t.Errorf("subscriber saw %d posts, want 1", got)
	}
}

func TestWorkerDeliversToALoopbackRangeAnOperatorAllowed(t *testing.T) {
	t.Parallel()

	subscriber, posts := countingSubscriber(t)

	settled, _ := deliverOnce(t, subscriber.URL+"/hook", loopback)

	if settled.Status != webhook.StatusDelivered {
		t.Errorf("status = %q, want delivered: %s", settled.Status, settled.LastError)
	}
	if got := posts.Load(); got != 1 {
		t.Errorf("subscriber saw %d posts, want 1", got)
	}
}

func TestWorkerDeliversToAHostEntryAnOperatorAllowed(t *testing.T) {
	t.Parallel()

	subscriber, posts := countingSubscriber(t)
	port := portOf(t, subscriber)
	guard := webhook.AddressGuard{Allowed: webhook.AllowList{Hosts: []string{"localhost:" + port}}}

	settled, _ := deliverOnce(t, "http://localhost:"+port+"/hook", guard)

	if settled.Status != webhook.StatusDelivered {
		t.Errorf("status = %q, want delivered: %s", settled.Status, settled.LastError)
	}
	if got := posts.Load(); got != 1 {
		t.Errorf("subscriber saw %d posts, want 1", got)
	}
}

func TestWorkerRefusesAMetadataAddressBehindAnAllowedHostEntry(t *testing.T) {
	t.Parallel()

	guard := webhook.AddressGuard{
		Allowed:  webhook.AllowList{Hosts: []string{"metadata.example.com:80"}},
		Resolver: fakeResolver(map[string]netip.Addr{"metadata.example.com": netip.MustParseAddr("169.254.169.254")}),
	}

	settled, logged := deliverOnce(t, "http://metadata.example.com/latest", guard)

	wantRefused(t, settled, logged, "metadata.example.com")
	if !strings.Contains(settled.LastError, "169.254.169.254:80") {
		t.Errorf("last_error = %q, want the resolved metadata address refused", settled.LastError)
	}
}

func TestWorkerIgnoresTheProxySettings(t *testing.T) {
	proxy, proxied := countingSubscriber(t)
	subscriber, posts := countingSubscriber(t)
	for _, name := range []string{"HTTP_PROXY", "http_proxy"} {
		t.Setenv(name, proxy.URL)
	}
	for _, name := range []string{"NO_PROXY", "no_proxy"} {
		t.Setenv(name, "")
	}
	guard := loopback
	guard.Resolver = fakeResolver(map[string]netip.Addr{"subscriber.example.com": netip.MustParseAddr("127.0.0.1")})

	settled, _ := deliverOnce(t, "http://subscriber.example.com:"+portOf(t, subscriber)+"/hook", guard)

	if got := proxied.Load(); got != 0 {
		t.Errorf("the proxy saw %d posts, want every proxy setting ignored", got)
	}
	if got := posts.Load(); got != 1 || settled.Status != webhook.StatusDelivered {
		t.Errorf("subscriber saw %d posts settled %q, want one delivered straight to it", got, settled.Status)
	}
}
