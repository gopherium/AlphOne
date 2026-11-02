// SPDX-License-Identifier: Elastic-2.0

package whatsapp

import (
	"errors"
	"fmt"
	"strings"
	"testing"
)

// trustingFetcher returns a fetcher over the stub that trusts the certificate of every TLS stub.
func trustingFetcher(t *testing.T, p *Plugin, stub *graphStub) *mediaFetcher {
	t.Helper()
	f := newTestFetcher(t, p, stub.server.URL, 1<<20)
	f.client.Transport = trustingTransport(t)
	return f
}

func TestALastErrorQuotesNoAddress(t *testing.T) {
	t.Parallel()

	binary := []byte("media-bytes")
	signed := "cdn-user:cdn-password@127.0.0.1:1/x?access_token=EAAG-in-location&hash=signed-in-address"
	broken := "https://cdn-user:cdn-password@127.0.0.1:1/%zz"
	tests := map[string]struct {
		start     func(*testing.T, []byte) *graphStub
		configure func(*graphStub)
		reason    string
	}{
		"a download redirect off the scheme": {
			start:     newTLSGraphStub,
			configure: func(s *graphStub) { s.binaryRedirect = "http://" + signed },
			reason:    "leaves the scheme",
		},
		"a metadata redirect off the scheme": {
			start:     newTLSGraphStub,
			configure: func(s *graphStub) { s.metadataRedirect = "http://" + signed },
			reason:    "leaves the scheme",
		},
		"a download redirect that does not parse": {
			start:     newTLSGraphStub,
			configure: func(s *graphStub) { s.binaryRedirect = broken },
			reason:    "media download failed",
		},
		"a metadata redirect that does not parse": {
			start:     newTLSGraphStub,
			configure: func(s *graphStub) { s.metadataRedirect = broken },
			reason:    "media metadata request failed",
		},
		"a download address that does not parse": {
			start:     newTLSGraphStub,
			configure: func(s *graphStub) { s.urlOverride = broken },
			reason:    "address does not parse",
		},
		"an unreachable address on the scheme": {
			start:     newGraphStub,
			configure: func(s *graphStub) { s.urlOverride = "http://" + signed },
			reason:    "media download failed",
		},
	}
	for testName, test := range tests {
		t.Run(testName, func(t *testing.T) {
			t.Parallel()

			p := newMigratedPlugin(t)
			stub := test.start(t, binary)
			test.configure(stub)
			f := trustingFetcher(t, p, stub)
			messageID := seedPendingImage(t, p, "wamid.quoted", shaOf(binary))

			f.sweepOnce(t.Context())

			wantRescheduled(t, p, messageID, test.reason)
			stored := *mediaState(t, p, messageID).lastError
			for _, quoted := range []string{"cdn-user", "cdn-password", "EAAG-in-location", "signed-in-address", "127.0.0.1"} {
				if strings.Contains(stored, quoted) {
					t.Errorf("last_error = %q, want it to leave out %q", stored, quoted)
				}
			}
		})
	}
}

func TestFetcherRefusesAnAddressOffTheSchemeOfTheGraphURL(t *testing.T) {
	t.Parallel()

	binary := []byte("media-bytes")
	tests := map[string]struct {
		address string
		reason  string
	}{
		"a plain http address":           {address: "http://{host}/binary", reason: "leaves the scheme"},
		"another scheme":                 {address: "ftp://{host}/binary", reason: "leaves the scheme"},
		"an address with no scheme":      {address: "//{host}/binary", reason: "leaves the scheme"},
		"a bare path":                    {address: "/binary", reason: "leaves the scheme"},
		"an empty address":               {address: "", reason: "leaves the scheme"},
		"an address that does not parse": {address: "http://{host}/%zz", reason: "does not parse"},
	}
	for testName, test := range tests {
		t.Run(testName, func(t *testing.T) {
			t.Parallel()

			p := newMigratedPlugin(t)
			plain := newGraphStub(t, binary)
			secure := newTLSGraphStub(t, binary)
			address := strings.ReplaceAll(test.address, "{host}", plain.server.Listener.Addr().String())
			secure.metadataBody = fmt.Sprintf(`{"url": %q, "mime_type": "image/jpeg", "file_size": 11}`, address)
			f := trustingFetcher(t, p, secure)
			messageID := seedPendingImage(t, p, "wamid.refused", shaOf(binary))

			f.sweepOnce(t.Context())

			wantRescheduled(t, p, messageID, test.reason)
			if heard := plain.headers(); len(heard) != 0 {
				t.Errorf("the plain stub heard %q, want no connection and no bearer", heard)
			}
			if _, binaryHits := secure.hits(); binaryHits != 0 {
				t.Errorf("binary downloads = %d, want none for a refused address", binaryHits)
			}
		})
	}
}

func TestARefusedAddressExpiresInsideTheRetryWindow(t *testing.T) {
	t.Parallel()

	p := newMigratedPlugin(t)
	binary := []byte("media-bytes")
	plain := newGraphStub(t, binary)
	secure := newTLSGraphStub(t, binary)
	secure.urlOverride = plain.server.URL + "/binary"
	f := trustingFetcher(t, p, secure)
	messageID := seedPendingImage(t, p, "wamid.expiring", shaOf(binary))
	f.sweepOnce(t.Context())
	wantRescheduled(t, p, messageID, "leaves the scheme")
	if _, err := p.pool.Exec(t.Context(),
		`UPDATE plugin_whatsapp.media
		SET created_at = now() - interval '8 days', next_attempt_at = now() - interval '1 second'
		WHERE message_id = $1`,
		messageID,
	); err != nil {
		t.Fatalf("backdating media row: %v", err)
	}

	f.sweepOnce(t.Context())

	wantFailed(t, p, messageID, "expired")
}

func TestFetcherDownloadsAnHTTPSAddressOnAnHTTPSGraphURL(t *testing.T) {
	t.Parallel()

	p := newMigratedPlugin(t)
	binary := []byte("media-bytes")
	secure := newTLSGraphStub(t, binary)
	f := trustingFetcher(t, p, secure)
	messageID := seedPendingImage(t, p, "wamid.secure", shaOf(binary))

	f.sweepOnce(t.Context())

	if state := mediaState(t, p, messageID); state.status != "stored" {
		t.Fatalf("status = %q, want %q", state.status, "stored")
	}
	if _, binaryHits := secure.hits(); binaryHits != 1 {
		t.Errorf("binary downloads = %d, want 1", binaryHits)
	}
	wantEveryBearer(t, secure, "test-token")
}

func TestFetcherRefusesARedirectOffTheSchemeOfTheGraphURL(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		start  func(*testing.T, []byte) *graphStub
		target func(*testing.T, []byte) *graphStub
		follow func(from *graphStub, to string)
	}{
		"a TLS stub sending the download to plain http": {
			start:  newTLSGraphStub,
			target: newGraphStub,
			follow: func(from *graphStub, to string) { from.binaryRedirect = to },
		},
		"a plain stub sending the download to TLS": {
			start:  newGraphStub,
			target: newTLSGraphStub,
			follow: func(from *graphStub, to string) { from.binaryRedirect = to },
		},
		"a TLS stub sending the metadata request to plain http": {
			start:  newTLSGraphStub,
			target: newGraphStub,
			follow: func(from *graphStub, to string) { from.metadataRedirect = to },
		},
	}
	for testName, test := range tests {
		t.Run(testName, func(t *testing.T) {
			t.Parallel()

			p := newMigratedPlugin(t)
			binary := []byte("media-bytes")
			start, target := test.start(t, binary), test.target(t, binary)
			test.follow(start, target.server.URL+"/binary")
			f := trustingFetcher(t, p, start)
			messageID := seedPendingImage(t, p, "wamid.redirected", shaOf(binary))

			f.sweepOnce(t.Context())

			wantRescheduled(t, p, messageID, "leaves the scheme")
			if heard := target.headers(); len(heard) != 0 {
				t.Errorf("the redirect's target heard %q, want no connection and no bearer", heard)
			}
		})
	}
}

func TestFetcherFollowsARedirectOnTheSchemeOfTheGraphURL(t *testing.T) {
	t.Parallel()

	tests := map[string]func(*testing.T, []byte) *graphStub{
		"plain http": newGraphStub,
		"TLS":        newTLSGraphStub,
	}
	for testName, start := range tests {
		t.Run(testName, func(t *testing.T) {
			t.Parallel()

			p := newMigratedPlugin(t)
			binary := []byte("media-bytes")
			first, second := start(t, []byte("not this file")), start(t, binary)
			first.binaryRedirect = second.server.URL + "/binary"
			f := trustingFetcher(t, p, first)
			messageID := seedPendingImage(t, p, "wamid.moved", shaOf(binary))

			f.sweepOnce(t.Context())

			if state := mediaState(t, p, messageID); state.status != "stored" {
				t.Fatalf("status = %q, want %q", state.status, "stored")
			}
		})
	}
}

func TestFetcherStopsFollowingAtTheTenthRedirect(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		hops   int
		status string
	}{
		"nine hops arrive": {hops: 9, status: "stored"},
		"ten hops stop":    {hops: 10, status: "pending"},
		"eleven hops stop": {hops: 11, status: "pending"},
	}
	for testName, test := range tests {
		t.Run(testName, func(t *testing.T) {
			t.Parallel()

			p := newMigratedPlugin(t)
			binary := []byte("media-bytes")
			stub := newGraphStub(t, binary)
			stub.hops = test.hops
			f := newTestFetcher(t, p, stub.server.URL, 1<<20)
			messageID := seedPendingImage(t, p, "wamid.hops", shaOf(binary))

			f.sweepOnce(t.Context())

			state := mediaState(t, p, messageID)
			if state.status != test.status {
				t.Fatalf("status = %q, want %q", state.status, test.status)
			}
			if state.status == "pending" {
				wantRescheduled(t, p, messageID, "redirects")
			}
			if _, binaryHits := stub.hits(); binaryHits != 10 {
				t.Errorf("binary requests = %d, want 10, the first and nine followed", binaryHits)
			}
		})
	}
}

func TestDownloadRefusesEveryAddressWhenTheGraphURLNamesNoScheme(t *testing.T) {
	t.Parallel()

	tests := map[string]string{
		"a base URL with no scheme":    "graph.example.com/v23.0",
		"a base URL that cannot parse": "::",
	}
	for testName, baseURL := range tests {
		t.Run(testName, func(t *testing.T) {
			t.Parallel()

			f := newMediaFetcher(&store{}, newBroadcaster(), mediaFetcherConfig{baseURL: baseURL})

			_, err := f.download(t.Context(), credentials{}, "/binary")

			if !errors.Is(err, errMediaScheme) {
				t.Errorf("download() error = %v, want %v", err, errMediaScheme)
			}
		})
	}
}
