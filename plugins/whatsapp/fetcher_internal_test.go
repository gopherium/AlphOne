// SPDX-License-Identifier: Elastic-2.0

package whatsapp

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/gopherium/alphone/sdk"
)

// waitFor blocks until cond holds, failing the test after two seconds.
func waitFor(t *testing.T, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatal("condition not met within 2s")
}

type graphStub struct {
	mu               sync.Mutex
	server           *httptest.Server
	binary           []byte
	mimeType         string
	fileSize         int64
	metadataCode     int
	metadataBody     string
	metadataRedirect string
	binaryCode       int
	binaryRedirect   string
	hops             int
	urlOverride      string
	truncate         bool
	authHeaders      []string
	requestURIs      []string
	metadataHits     int
	binaryHits       int
}

// unstartedGraphStub returns a stub graph server's answers with no server behind them yet.
func unstartedGraphStub(binary []byte) *graphStub {
	return &graphStub{
		binary:       binary,
		mimeType:     "image/jpeg",
		fileSize:     int64(len(binary)),
		metadataCode: http.StatusOK,
		binaryCode:   http.StatusOK,
	}
}

// newGraphStub starts a stub graph server answering media metadata and the binary, closed at cleanup.
func newGraphStub(t *testing.T, binary []byte) *graphStub {
	t.Helper()
	s := unstartedGraphStub(binary)
	s.server = httptest.NewServer(http.HandlerFunc(s.handle))
	t.Cleanup(s.server.Close)
	return s
}

// newTLSGraphStub starts a stub graph server behind TLS, closed at cleanup.
func newTLSGraphStub(t *testing.T, binary []byte) *graphStub {
	t.Helper()
	s := unstartedGraphStub(binary)
	s.server = httptest.NewTLSServer(http.HandlerFunc(s.handle))
	t.Cleanup(s.server.Close)
	return s
}

// trustingTransport returns a transport trusting the certificate every TLS stub serves, closed at cleanup.
func trustingTransport(t *testing.T) http.RoundTripper {
	t.Helper()
	return newTLSGraphStub(t, nil).server.Client().Transport
}

// handle records the request authorization and answers either the media metadata or the binary.
func (s *graphStub) handle(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.authHeaders = append(s.authHeaders, r.Header.Get("Authorization"))
	s.requestURIs = append(s.requestURIs, r.RequestURI)
	if strings.HasPrefix(r.URL.Path, "/binary") {
		s.binaryHits++
		if s.hops > 0 {
			s.hops--
			http.Redirect(w, r, "/binary", http.StatusFound)
			return
		}
		if s.binaryRedirect != "" {
			http.Redirect(w, r, s.binaryRedirect, http.StatusFound)
			return
		}
		if s.binaryCode != http.StatusOK {
			w.WriteHeader(s.binaryCode)
			return
		}
		if s.truncate {
			w.Header().Set("Content-Length", "1000")
			_, _ = w.Write(s.binary[:1])
			return
		}
		_, _ = w.Write(s.binary)
		return
	}
	s.metadataHits++
	if s.metadataRedirect != "" {
		http.Redirect(w, r, s.metadataRedirect, http.StatusFound)
		return
	}
	if s.metadataCode != http.StatusOK {
		w.WriteHeader(s.metadataCode)
		return
	}
	if s.metadataBody != "" {
		_, _ = io.WriteString(w, s.metadataBody)
		return
	}
	downloadURL := s.urlOverride
	if downloadURL == "" {
		downloadURL = s.server.URL + "/binary"
	}
	_, _ = fmt.Fprintf(w, `{"url": %q, "mime_type": %q, "file_size": %d, "id": "MEDIA1"}`,
		downloadURL, s.mimeType, s.fileSize)
}

// hits returns how many metadata and binary requests the stub answered.
func (s *graphStub) hits() (int, int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.metadataHits, s.binaryHits
}

// headers returns a copy of the Authorization headers the stub received.
func (s *graphStub) headers() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]string(nil), s.authHeaders...)
}

// uris returns a copy of the request addresses the stub received.
func (s *graphStub) uris() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]string(nil), s.requestURIs...)
}

// shaOf returns the base64 encoded sha256 sum of the given bytes.
func shaOf(data []byte) string {
	sum := sha256.Sum256(data)
	return base64.StdEncoding.EncodeToString(sum[:])
}

// newTestFetcher gives the plugin environment credentials and returns a fetcher over its store and events.
func newTestFetcher(t *testing.T, p *Plugin, baseURL string, maxBytes int64) *mediaFetcher {
	t.Helper()
	p.envCredentials = credentials{phoneNumberID: "PN1", accessToken: "test-token"}
	return newMediaFetcher(p.store, p.events, mediaFetcherConfig{
		baseURL:     baseURL,
		credentials: p.credentialsFor,
		records:     p.recordsTraffic,
		maxBytes:    maxBytes,
		interval:    time.Hour,
		timeout:     time.Minute,
	})
}

// seedPendingImage stores one message with a pending media row and returns the message id.
func seedPendingImage(t *testing.T, p *Plugin, externalID, sha string) uuid.UUID {
	t.Helper()
	_, messageID := seedMessage(t, p, externalID)
	insertPendingMedia(t, p, messageID, mediaDescriptor{
		mediaID: "MEDIA1", mimeType: "application/octet-stream", sha256: sha,
	})
	return messageID
}

type mediaRowState struct {
	status    string
	attempts  int
	lastError *string
	data      []byte
	mimeType  string
	fileSize  *int64
}

// mediaState reads the media row the message names, failing the test when it cannot be read.
func mediaState(t *testing.T, p *Plugin, messageID uuid.UUID) mediaRowState {
	t.Helper()
	var s mediaRowState
	err := p.pool.QueryRow(t.Context(),
		`SELECT status, attempts, last_error, data, mime_type, file_size
		FROM plugin_whatsapp.media WHERE message_id = $1`,
		messageID,
	).Scan(&s.status, &s.attempts, &s.lastError, &s.data, &s.mimeType, &s.fileSize)
	if err != nil {
		t.Fatalf("loading media row: %v", err)
	}
	return s
}

// mediaStatus returns the status of the media row the message names, empty when there is none.
func mediaStatus(p *Plugin, messageID uuid.UUID) string {
	var status string
	_ = p.pool.QueryRow(context.Background(),
		`SELECT status FROM plugin_whatsapp.media WHERE message_id = $1`, messageID,
	).Scan(&status)
	return status
}

func TestFetcherStoresPendingMedia(t *testing.T) {
	t.Parallel()

	p := newMigratedPlugin(t)
	binary := []byte("media-bytes")
	stub := newGraphStub(t, binary)
	f := newTestFetcher(t, p, stub.server.URL, 1<<20)
	messageID := seedPendingImage(t, p, "wamid.happy", shaOf(binary))
	subscription := p.events.subscribe(sdk.DefaultTenantID)
	defer p.events.unsubscribe(subscription)

	f.sweepOnce(t.Context())

	state := mediaState(t, p, messageID)
	if state.status != "stored" {
		t.Fatalf("status = %q, want %q", state.status, "stored")
	}
	if string(state.data) != string(binary) {
		t.Errorf("data = %q, want %q", state.data, binary)
	}
	if state.mimeType != "image/jpeg" {
		t.Errorf("mime_type = %q, want the metadata value %q", state.mimeType, "image/jpeg")
	}
	if state.fileSize == nil || *state.fileSize != int64(len(binary)) {
		t.Errorf("file_size = %v, want %d", state.fileSize, len(binary))
	}
	select {
	case <-subscription:
	default:
		t.Error("no event broadcast after the media was stored")
	}
	for _, header := range stub.headers() {
		if header != "Bearer test-token" {
			t.Errorf("Authorization = %q, want %q", header, "Bearer test-token")
		}
	}
}

// seededAcmeImage stores a pending image under a fresh tenant with its own credentials.
func seededAcmeImage(t *testing.T, p *Plugin) (uuid.UUID, uuid.UUID) {
	t.Helper()
	p.UseEncrypter(&boundEncrypter{})
	acme := seededTenant(t, p, "Acme")
	mine := sdk.WithTenant(t.Context(), acme)
	connected(t, p, mine, "555000444", "EAAG-acme-token")
	conversationID := seededConversation(t, p, "wamid.acme", acme)
	messageID := uuid.Must(uuid.NewV7())
	if _, err := p.pool.Exec(t.Context(),
		`INSERT INTO plugin_whatsapp.messages (id, conversation_id, external_id, direction, content,
			content_type, sent_at, raw, created_at, tenant_id)
		VALUES ($1, $2, 'wamid.acme', 'inbound', '', 'image', now(), '{}', now(), $3)`,
		messageID, conversationID, acme,
	); err != nil {
		t.Fatalf("inserting message: %v", err)
	}
	if err := insertMediaPending(mine, p.pool, messageID, mediaDescriptor{
		mediaID: "MEDIA1", mimeType: "image/jpeg", sha256: shaOf([]byte("media-bytes")),
	}); err != nil {
		t.Fatalf("insertMediaPending() error = %v, want nil", err)
	}
	return acme, messageID
}

func TestFetcherUsesTheRowTenantsCredentials(t *testing.T) {
	t.Parallel()

	p := newMigratedPlugin(t)
	binary := []byte("media-bytes")
	stub := newGraphStub(t, binary)
	f := newTestFetcher(t, p, stub.server.URL, 1<<20)
	_, messageID := seededAcmeImage(t, p)

	f.sweepOnce(t.Context())

	if state := mediaState(t, p, messageID); state.status != "stored" {
		t.Fatalf("status = %q, want %q", state.status, "stored")
	}
	headers := stub.headers()
	if len(headers) == 0 {
		t.Fatal("no graph requests recorded, want the download performed")
	}
	for _, header := range headers {
		if header != "Bearer EAAG-acme-token" {
			t.Errorf("Authorization = %q, want the row tenant's token", header)
		}
	}
}

func TestFetcherReschedulesARowWhoseTenantHasNoCredentials(t *testing.T) {
	t.Parallel()

	p := newEncryptingPlugin(t)
	stub := newGraphStub(t, []byte("media-bytes"))
	f := newTestFetcher(t, p, stub.server.URL, 1<<20)
	bare := seededTenant(t, p, "Globex")
	conversationID := seededConversation(t, p, "wamid.bare", bare)
	messageID := uuid.Must(uuid.NewV7())
	if _, err := p.pool.Exec(t.Context(),
		`INSERT INTO plugin_whatsapp.messages (id, conversation_id, external_id, direction, content,
			content_type, sent_at, raw, created_at, tenant_id)
		VALUES ($1, $2, 'wamid.bare', 'inbound', '', 'image', now(), '{}', now(), $3)`,
		messageID, conversationID, bare,
	); err != nil {
		t.Fatalf("inserting message: %v", err)
	}
	if err := insertMediaPending(sdk.WithTenant(t.Context(), bare), p.pool, messageID, mediaDescriptor{
		mediaID: "MEDIA1", mimeType: "image/jpeg", sha256: shaOf([]byte("media-bytes")),
	}); err != nil {
		t.Fatalf("insertMediaPending() error = %v, want nil", err)
	}

	f.sweepOnce(t.Context())

	state := mediaState(t, p, messageID)
	if state.status != "pending" || state.attempts != 1 {
		t.Fatalf("row = (%q, %d), want still pending after one attempt", state.status, state.attempts)
	}
	if state.lastError == nil || !strings.Contains(*state.lastError, "credentials") {
		t.Errorf("last_error = %v, want the missing credentials named", state.lastError)
	}
	if metadataHits, binaryHits := stub.hits(); metadataHits != 0 || binaryHits != 0 {
		t.Errorf("graph hits = (%d, %d), want none without credentials", metadataHits, binaryHits)
	}
}

// seededImageOn stores a pending image on a conversation that came in on a number of the tenant.
func seededImageOn(t *testing.T, p *Plugin, tenantID uuid.UUID, number string) uuid.UUID {
	t.Helper()
	now := time.Now().UTC()
	conversationID := datedConversation(t, p, tenantID, seededContact(t, p, tenantID), &number, now, now)
	messageID := oneMessage(t, p, tenantID, conversationID, "wamid."+number)
	if err := insertMediaPending(sdk.WithTenant(t.Context(), tenantID), p.pool, messageID, mediaDescriptor{
		mediaID: "MEDIA1", mimeType: "image/jpeg", sha256: shaOf([]byte("media-bytes")),
	}); err != nil {
		t.Fatalf("insertMediaPending() error = %v, want nil", err)
	}
	return messageID
}

// queuedImages stores count pending images on one conversation of the tenant, oldest first, and returns their ids.
func queuedImages(t *testing.T, p *Plugin, tenantID uuid.UUID, count int) []uuid.UUID {
	t.Helper()
	now := time.Now().UTC()
	conversationID := datedConversation(t, p, tenantID, seededContact(t, p, tenantID), nil, now, now)
	queued := make([]uuid.UUID, count)
	for i := range queued {
		queued[i] = oneMessage(t, p, tenantID, conversationID, fmt.Sprintf("wamid.queued.%d", i))
		if err := insertMediaPending(sdk.WithTenant(t.Context(), tenantID), p.pool, queued[i], mediaDescriptor{
			mediaID: "MEDIA1", mimeType: "image/jpeg", sha256: shaOf([]byte("media-bytes")),
		}); err != nil {
			t.Fatalf("insertMediaPending() error = %v, want nil", err)
		}
	}
	return queued
}

// tenantGate answers whether a tenant records traffic from the function it holds.
type tenantGate func(uuid.UUID) (bool, error)

// AcceptsMachineTraffic answers what the function says of the tenant.
func (g tenantGate) AcceptsMachineTraffic(_ context.Context, tenantID uuid.UUID) (bool, error) {
	return g(tenantID)
}

func TestFetcherReachesAnOpenTenantPastAClosedTenantsRows(t *testing.T) {
	t.Parallel()

	tests := map[string]func() (bool, error){
		"a closed tenant": func() (bool, error) { return false, nil },
		"a failing gate":  func() (bool, error) { return false, errors.New("the gate is down") },
	}
	for testName, closing := range tests {
		t.Run(testName, func(t *testing.T) {
			t.Parallel()

			p := newEncryptingPlugin(t)
			stub := newGraphStub(t, []byte("media-bytes"))
			f := newTestFetcher(t, p, stub.server.URL, 1<<20)
			closed, open := seededTenant(t, p, "Acme"), seededTenant(t, p, "Globex")
			p.gate = tenantGate(func(tenantID uuid.UUID) (bool, error) {
				if tenantID == closed {
					return closing()
				}
				return true, nil
			})
			connected(t, p, sdk.WithTenant(t.Context(), open), "5550002", "EAAG-open-token")
			leftAlone := queuedImages(t, p, closed, mediaSweepBatch)
			messageID := seededImageOn(t, p, open, "5550002")

			f.sweepOnce(t.Context())

			if state := mediaState(t, p, messageID); state.status != "stored" {
				t.Fatalf("the open tenant's row = %q, want %q", state.status, "stored")
			}
			for _, id := range leftAlone {
				if state := mediaState(t, p, id); state.status != "pending" || state.attempts != 0 {
					t.Errorf("the closed tenant's row = (%q, %d), want it pending and untried", state.status, state.attempts)
				}
			}
			for _, header := range stub.headers() {
				if header != "Bearer EAAG-open-token" {
					t.Errorf("Authorization = %q, want only the open tenant's token", header)
				}
			}
		})
	}
}

func TestFetcherKeepsAMediaIDInsideOnePathSegment(t *testing.T) {
	t.Parallel()

	p := newMigratedPlugin(t)
	binary := []byte("media-bytes")
	stub := newGraphStub(t, binary)
	f := newTestFetcher(t, p, stub.server.URL, 1<<20)
	_, messageID := seedMessage(t, p, "wamid.steered")
	insertPendingMedia(t, p, messageID, mediaDescriptor{
		mediaID: "../424242?fields=privacy_policy_url.as(url)&ignored=", mimeType: "image/jpeg", sha256: shaOf(binary),
	})

	f.sweepOnce(t.Context())

	requests := stub.uris()
	if len(requests) == 0 {
		t.Fatal("no graph requests recorded, want the metadata asked for")
	}
	want := "/..%2F424242%3Ffields=privacy_policy_url.as%28url%29&ignored=?phone_number_id=PN1"
	if requests[0] != want {
		t.Errorf("metadata request = %q, want %q", requests[0], want)
	}
}

func TestFetcherUsesTheTokenOfTheNumberItsConversationCameInOn(t *testing.T) {
	t.Parallel()

	p := newEncryptingPlugin(t)
	stub := newGraphStub(t, []byte("media-bytes"))
	f := newTestFetcher(t, p, stub.server.URL, 1<<20)
	acme := seededTenant(t, p, "Acme")
	mine := sdk.WithTenant(t.Context(), acme)
	connected(t, p, mine, "5550001", "EAAG-first-token")
	connected(t, p, mine, "5550002", "EAAG-second-token")
	messageID := seededImageOn(t, p, acme, "5550002")

	f.sweepOnce(t.Context())

	if state := mediaState(t, p, messageID); state.status != "stored" {
		t.Fatalf("status = %q, want %q", state.status, "stored")
	}
	for _, header := range stub.headers() {
		if header != "Bearer EAAG-second-token" {
			t.Errorf("Authorization = %q, want the token of the conversation's number", header)
		}
	}
}

func TestFetcherUsesTheEnvironmentTokenForAConversationOnNoNumber(t *testing.T) {
	t.Parallel()

	p := newEncryptingPlugin(t)
	binary := []byte("media-bytes")
	stub := newGraphStub(t, binary)
	f := newTestFetcher(t, p, stub.server.URL, 1<<20)
	connected(t, p, t.Context(), "5550001", "EAAG-stored-token")
	messageID := seedPendingImage(t, p, "wamid.envnumber", shaOf(binary))

	f.sweepOnce(t.Context())

	if state := mediaState(t, p, messageID); state.status != "stored" {
		t.Fatalf("status = %q, want %q", state.status, "stored")
	}
	for _, header := range stub.headers() {
		if header != "Bearer test-token" {
			t.Errorf("Authorization = %q, want the environment token", header)
		}
	}
}

func TestFetcherReschedulesARowOnARemovedNumber(t *testing.T) {
	t.Parallel()

	p := newEncryptingPlugin(t)
	stub := newGraphStub(t, []byte("media-bytes"))
	f := newTestFetcher(t, p, stub.server.URL, 1<<20)
	acme := seededTenant(t, p, "Acme")
	connected(t, p, sdk.WithTenant(t.Context(), acme), "5550001", "EAAG-first-token")
	messageID := seededImageOn(t, p, acme, "5550002")

	f.sweepOnce(t.Context())

	state := mediaState(t, p, messageID)
	if state.status != "pending" || state.attempts != 1 {
		t.Fatalf("row = (%q, %d), want still pending after one attempt", state.status, state.attempts)
	}
	if state.lastError == nil || !strings.Contains(*state.lastError, "no longer connected") {
		t.Errorf("last_error = %v, want the removed number named", state.lastError)
	}
	if metadataHits, binaryHits := stub.hits(); metadataHits != 0 || binaryHits != 0 {
		t.Errorf("graph hits = (%d, %d), want none with another number's token", metadataHits, binaryHits)
	}
}

// closedFetcherGate refuses machine traffic for every tenant.
type closedFetcherGate struct{}

// AcceptsMachineTraffic refuses every tenant.
func (closedFetcherGate) AcceptsMachineTraffic(context.Context, uuid.UUID) (bool, error) {
	return false, nil
}

func TestFetcherLeavesARowAlonePastTheGraceWindow(t *testing.T) {
	t.Parallel()

	p := newMigratedPlugin(t)
	binary := []byte("media-bytes")
	stub := newGraphStub(t, binary)
	f := newTestFetcher(t, p, stub.server.URL, 1<<20)
	p.gate = closedFetcherGate{}
	messageID := seedPendingImage(t, p, "wamid.closed", shaOf(binary))

	f.sweepOnce(t.Context())

	if state := mediaState(t, p, messageID); state.status != "pending" {
		t.Errorf("status = %q, want the row left pending once the grace window closed", state.status)
	}
	if metadataHits, binaryHits := stub.hits(); metadataHits != 0 || binaryHits != 0 {
		t.Errorf("graph hits = (%d, %d), want none for a closed tenant", metadataHits, binaryHits)
	}
}

func TestFetcherLeavesSettledRowsAlone(t *testing.T) {
	t.Parallel()

	p := newMigratedPlugin(t)
	binary := []byte("media-bytes")
	stub := newGraphStub(t, binary)
	f := newTestFetcher(t, p, stub.server.URL, 1<<20)
	seedPendingImage(t, p, "wamid.once", shaOf(binary))
	f.sweepOnce(t.Context())

	f.sweepOnce(t.Context())

	metadataHits, binaryHits := stub.hits()
	if metadataHits != 1 || binaryHits != 1 {
		t.Errorf("graph hits = (%d, %d), want (1, 1)", metadataHits, binaryHits)
	}
}

func TestFetcherReschedulesOnGraphErrors(t *testing.T) {
	t.Parallel()

	binary := []byte("media-bytes")
	tests := map[string]struct {
		configure func(t *testing.T, s *graphStub, f *mediaFetcher)
		reason    string
	}{
		"metadata status 500": {
			configure: func(_ *testing.T, s *graphStub, _ *mediaFetcher) { s.metadataCode = http.StatusInternalServerError },
			reason:    "500",
		},
		"metadata status 401": {
			configure: func(_ *testing.T, s *graphStub, _ *mediaFetcher) { s.metadataCode = http.StatusUnauthorized },
			reason:    "401",
		},
		"metadata garbage": {
			configure: func(_ *testing.T, s *graphStub, _ *mediaFetcher) { s.metadataBody = "not-json" },
			reason:    "decode",
		},
		"metadata unreachable": {
			configure: func(_ *testing.T, s *graphStub, _ *mediaFetcher) { s.server.Close() },
			reason:    "metadata",
		},
		"metadata request unbuildable": {
			configure: func(_ *testing.T, _ *graphStub, f *mediaFetcher) { f.baseURL = "::" },
			reason:    "does not parse",
		},
		"download status 404": {
			configure: func(_ *testing.T, s *graphStub, _ *mediaFetcher) { s.binaryCode = http.StatusNotFound },
			reason:    "404",
		},
		"download unreachable": {
			configure: func(_ *testing.T, s *graphStub, _ *mediaFetcher) { s.urlOverride = "http://127.0.0.1:1/binary" },
			reason:    "download",
		},
		"download request unbuildable": {
			configure: func(_ *testing.T, s *graphStub, _ *mediaFetcher) { s.urlOverride = "::" },
			reason:    "does not parse",
		},
		"download truncated": {
			configure: func(_ *testing.T, s *graphStub, _ *mediaFetcher) { s.truncate = true },
			reason:    "download",
		},
		"sha mismatch": {
			configure: func(_ *testing.T, s *graphStub, _ *mediaFetcher) { s.binary = []byte("tampered") },
			reason:    "sha256",
		},
	}

	for testName, tc := range tests {
		t.Run(testName, func(t *testing.T) {
			t.Parallel()

			p := newMigratedPlugin(t)
			stub := newGraphStub(t, binary)
			f := newTestFetcher(t, p, stub.server.URL, 1<<20)
			messageID := seedPendingImage(t, p, "wamid.retry", shaOf(binary))
			subscription := p.events.subscribe(sdk.DefaultTenantID)
			defer p.events.unsubscribe(subscription)
			tc.configure(t, stub, f)

			f.sweepOnce(t.Context())

			state := mediaState(t, p, messageID)
			if state.status != "pending" {
				t.Fatalf("status = %q, want still pending", state.status)
			}
			if state.attempts != 1 {
				t.Errorf("attempts = %d, want 1", state.attempts)
			}
			if state.lastError == nil || !strings.Contains(*state.lastError, tc.reason) {
				t.Errorf("last_error = %v, want it to contain %q", state.lastError, tc.reason)
			}
			select {
			case <-subscription:
				t.Error("a retry broadcast an event, want none")
			default:
			}
			pending, err := p.store.claimDueMedia(t.Context(), time.Now().UTC(), 10, noClosedTenants)
			if err != nil {
				t.Fatalf("claimDueMedia() error = %v, want nil", err)
			}
			if len(pending) != 0 {
				t.Errorf("row still due immediately after a retry, want a deferred next attempt")
			}
		})
	}
}

// wantRescheduled fails the test unless the row waits for its next attempt under a reason holding the given text.
func wantRescheduled(t *testing.T, p *Plugin, messageID uuid.UUID, reason string) {
	t.Helper()
	state := mediaState(t, p, messageID)
	if state.status != "pending" || state.attempts != 1 {
		t.Fatalf("row = (%q, %d), want still pending after one attempt", state.status, state.attempts)
	}
	if state.lastError == nil || !strings.Contains(*state.lastError, reason) {
		t.Errorf("last_error = %v, want it to contain %q", state.lastError, reason)
	}
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
			f := newTestFetcher(t, p, stub.server.URL, 1<<20)
			f.client.Transport = trustingTransport(t)
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
			f := newTestFetcher(t, p, secure.server.URL, 1<<20)
			f.client.Transport = trustingTransport(t)
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
	f := newTestFetcher(t, p, secure.server.URL, 1<<20)
	f.client.Transport = trustingTransport(t)
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

	state := mediaState(t, p, messageID)
	if state.status != "failed" || state.lastError == nil || *state.lastError != "expired" {
		t.Fatalf("row = (%q, %v), want (failed, expired)", state.status, state.lastError)
	}
}

func TestFetcherDownloadsAnHTTPSAddressOnAnHTTPSGraphURL(t *testing.T) {
	t.Parallel()

	p := newMigratedPlugin(t)
	binary := []byte("media-bytes")
	secure := newTLSGraphStub(t, binary)
	f := newTestFetcher(t, p, secure.server.URL, 1<<20)
	f.client.Transport = trustingTransport(t)
	messageID := seedPendingImage(t, p, "wamid.secure", shaOf(binary))

	f.sweepOnce(t.Context())

	if state := mediaState(t, p, messageID); state.status != "stored" {
		t.Fatalf("status = %q, want %q", state.status, "stored")
	}
	if _, binaryHits := secure.hits(); binaryHits != 1 {
		t.Errorf("binary downloads = %d, want 1", binaryHits)
	}
	for _, header := range secure.headers() {
		if header != "Bearer test-token" {
			t.Errorf("Authorization = %q, want %q", header, "Bearer test-token")
		}
	}
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
			f := newTestFetcher(t, p, start.server.URL, 1<<20)
			f.client.Transport = trustingTransport(t)
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
			f := newTestFetcher(t, p, first.server.URL, 1<<20)
			f.client.Transport = trustingTransport(t)
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

func TestFetcherStoresWhenShaIsUnverifiable(t *testing.T) {
	t.Parallel()

	p := newMigratedPlugin(t)
	binary := []byte("media-bytes")
	stub := newGraphStub(t, binary)
	f := newTestFetcher(t, p, stub.server.URL, 1<<20)
	messageID := seedPendingImage(t, p, "wamid.badsha", "!!!not-base64!!!")

	f.sweepOnce(t.Context())

	if state := mediaState(t, p, messageID); state.status != "stored" {
		t.Fatalf("status = %q, want stored despite the unverifiable hash", state.status)
	}
}

func TestFetcherFailsOversizedMediaWithoutDownloading(t *testing.T) {
	t.Parallel()

	p := newMigratedPlugin(t)
	binary := []byte("media-bytes")
	stub := newGraphStub(t, binary)
	stub.fileSize = 9
	f := newTestFetcher(t, p, stub.server.URL, 8)
	messageID := seedPendingImage(t, p, "wamid.big", shaOf(binary))
	subscription := p.events.subscribe(sdk.DefaultTenantID)
	defer p.events.unsubscribe(subscription)

	f.sweepOnce(t.Context())

	state := mediaState(t, p, messageID)
	if state.status != "failed" {
		t.Fatalf("status = %q, want %q", state.status, "failed")
	}
	if state.lastError == nil || *state.lastError != "too_large" {
		t.Errorf("last_error = %v, want too_large", state.lastError)
	}
	if _, binaryHits := stub.hits(); binaryHits != 0 {
		t.Errorf("binary downloads = %d, want 0 for an oversized declaration", binaryHits)
	}
	select {
	case <-subscription:
	default:
		t.Error("no event broadcast after the terminal failure")
	}
}

func TestFetcherFailsMediaLyingAboutItsSize(t *testing.T) {
	t.Parallel()

	p := newMigratedPlugin(t)
	binary := []byte("twenty-bytes-of-lies")
	stub := newGraphStub(t, binary)
	stub.fileSize = 4
	f := newTestFetcher(t, p, stub.server.URL, 8)
	messageID := seedPendingImage(t, p, "wamid.liar", shaOf(binary))

	f.sweepOnce(t.Context())

	state := mediaState(t, p, messageID)
	if state.status != "failed" || state.lastError == nil || *state.lastError != "too_large" {
		t.Fatalf("row = (%q, %v), want (failed, too_large)", state.status, state.lastError)
	}
}

func TestFetcherExpiresRowsPastTheRetryWindow(t *testing.T) {
	t.Parallel()

	p := newMigratedPlugin(t)
	stub := newGraphStub(t, []byte("media-bytes"))
	f := newTestFetcher(t, p, stub.server.URL, 1<<20)
	messageID := seedPendingImage(t, p, "wamid.old", shaOf([]byte("media-bytes")))
	if _, err := p.pool.Exec(t.Context(),
		`UPDATE plugin_whatsapp.media SET created_at = now() - interval '8 days' WHERE message_id = $1`,
		messageID,
	); err != nil {
		t.Fatalf("backdating media row: %v", err)
	}
	subscription := p.events.subscribe(sdk.DefaultTenantID)
	defer p.events.unsubscribe(subscription)

	f.sweepOnce(t.Context())

	state := mediaState(t, p, messageID)
	if state.status != "failed" || state.lastError == nil || *state.lastError != "expired" {
		t.Fatalf("row = (%q, %v), want (failed, expired)", state.status, state.lastError)
	}
	if metadataHits, binaryHits := stub.hits(); metadataHits != 0 || binaryHits != 0 {
		t.Errorf("graph hits = (%d, %d), want none for an expired row", metadataHits, binaryHits)
	}
	select {
	case <-subscription:
	default:
		t.Error("no event broadcast after the row expired")
	}
}

// staticCredentials answers the same credentials for every caller.
func staticCredentials(creds credentials) func(context.Context, string) (credentials, error) {
	return func(context.Context, string) (credentials, error) { return creds, nil }
}

func TestFetcherToleratesStoreFailures(t *testing.T) {
	t.Parallel()

	binary := []byte("media-bytes")
	row := pendingMediaRow{
		MessageID:      uuid.Must(uuid.NewV7()),
		ConversationID: uuid.Must(uuid.NewV7()),
		MediaID:        "MEDIA1",
		SHA256:         shaOf(binary),
		CreatedAt:      time.Now().UTC(),
	}

	t.Run("settling", func(t *testing.T) {
		t.Parallel()
		f := newMediaFetcher(&store{pool: newUnreachablePool(t)}, newBroadcaster(), mediaFetcherConfig{})
		old := row
		old.CreatedAt = time.Now().UTC().Add(-8 * 24 * time.Hour)

		f.fetchOne(t.Context(), old)
	})

	t.Run("rescheduling", func(t *testing.T) {
		t.Parallel()
		stub := newGraphStub(t, binary)
		stub.metadataCode = http.StatusInternalServerError
		f := newMediaFetcher(&store{pool: newUnreachablePool(t)}, newBroadcaster(), mediaFetcherConfig{
			baseURL:     stub.server.URL,
			credentials: staticCredentials(credentials{phoneNumberID: "PN1", accessToken: "test-token"}),
		})

		f.fetchOne(t.Context(), row)
	})

	t.Run("storing", func(t *testing.T) {
		t.Parallel()
		stub := newGraphStub(t, binary)
		f := newMediaFetcher(&store{pool: newUnreachablePool(t)}, newBroadcaster(), mediaFetcherConfig{
			baseURL:     stub.server.URL,
			credentials: staticCredentials(credentials{phoneNumberID: "PN1", accessToken: "test-token"}),
		})

		f.fetchOne(t.Context(), row)
	})
}

func TestFetcherSweepSurvivesClaimFailure(t *testing.T) {
	t.Parallel()

	f := newMediaFetcher(&store{pool: newUnreachablePool(t)}, newBroadcaster(), mediaFetcherConfig{})

	f.sweepOnce(t.Context())
}

func TestBackoffGrowsAndCaps(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		attempts int
		want     time.Duration
	}{
		"first retry":   {attempts: 0, want: 30 * time.Second},
		"second retry":  {attempts: 1, want: time.Minute},
		"third retry":   {attempts: 2, want: 2 * time.Minute},
		"seventh retry": {attempts: 6, want: 32 * time.Minute},
		"eighth retry":  {attempts: 7, want: time.Hour},
		"much later":    {attempts: 40, want: time.Hour},
	}

	for testName, tc := range tests {
		t.Run(testName, func(t *testing.T) {
			t.Parallel()

			if got := backoff(tc.attempts); got != tc.want {
				t.Errorf("backoff(%d) = %v, want %v", tc.attempts, got, tc.want)
			}
		})
	}
}

func TestFetcherStartSweepsImmediately(t *testing.T) {
	t.Parallel()

	p := newMigratedPlugin(t)
	binary := []byte("media-bytes")
	stub := newGraphStub(t, binary)
	f := newTestFetcher(t, p, stub.server.URL, 1<<20)
	messageID := seedPendingImage(t, p, "wamid.startup", shaOf(binary))

	f.Start()
	defer func() { _ = f.Stop(t.Context()) }()

	waitFor(t, func() bool { return mediaStatus(p, messageID) == "stored" })
}

func TestFetcherNudgeWakesTheLoop(t *testing.T) {
	t.Parallel()

	p := newMigratedPlugin(t)
	binary := []byte("media-bytes")
	stub := newGraphStub(t, binary)
	f := newTestFetcher(t, p, stub.server.URL, 1<<20)
	f.Start()
	defer func() { _ = f.Stop(t.Context()) }()
	waitFor(t, func() bool { return f.sweeps.Load() >= 1 })
	messageID := seedPendingImage(t, p, "wamid.nudged", shaOf(binary))

	f.poke()

	waitFor(t, func() bool { return mediaStatus(p, messageID) == "stored" })
}

func TestFetcherPokeNeverBlocksWithoutARunningLoop(t *testing.T) {
	t.Parallel()

	f := &mediaFetcher{nudge: make(chan struct{}, 1)}

	f.poke()
	f.poke()
}

func TestFetcherTickerSweeps(t *testing.T) {
	t.Parallel()

	p := newMigratedPlugin(t)
	binary := []byte("media-bytes")
	stub := newGraphStub(t, binary)
	p.envCredentials = credentials{phoneNumberID: "PN1", accessToken: "test-token"}
	f := newMediaFetcher(p.store, p.events, mediaFetcherConfig{
		baseURL:     stub.server.URL,
		credentials: p.credentialsFor,
		maxBytes:    1 << 20,
		interval:    25 * time.Millisecond,
		timeout:     time.Minute,
	})
	f.Start()
	defer func() { _ = f.Stop(t.Context()) }()
	waitFor(t, func() bool { return f.sweeps.Load() >= 1 })
	messageID := seedPendingImage(t, p, "wamid.ticked", shaOf(binary))

	waitFor(t, func() bool { return mediaStatus(p, messageID) == "stored" })
}

func TestFetcherStopWithoutStart(t *testing.T) {
	t.Parallel()

	f := newMediaFetcher(&store{}, newBroadcaster(), mediaFetcherConfig{})

	if err := f.Stop(t.Context()); err != nil {
		t.Errorf("Stop() without Start() = %v, want nil", err)
	}
}

func TestStopReturnsWhenItsContextEndsWhileASweepRuns(t *testing.T) {
	t.Parallel()

	p := newMigratedPlugin(t)
	seedPendingImage(t, p, "wamid.held", shaOf([]byte("media-bytes")))
	sweeping, release := context.WithCancel(context.Background())
	t.Cleanup(release)
	var holding atomic.Bool
	p.fetcher = newMediaFetcher(p.store, p.events, mediaFetcherConfig{
		records: func(context.Context, uuid.UUID) (bool, error) {
			conn, err := p.pool.Acquire(sweeping)
			if err != nil {
				return false, err
			}
			defer conn.Release()
			holding.Store(true)
			<-sweeping.Done()
			return false, nil
		},
	})
	if err := p.Start(t.Context()); err != nil {
		t.Fatalf("Start() error = %v, want nil", err)
	}
	waitFor(t, holding.Load)
	ctx, cancel := context.WithCancel(t.Context())
	stopped := make(chan error, 1)
	go func() { stopped <- p.Stop(ctx) }()

	cancel()

	waitFor(t, func() bool { return len(stopped) == 1 })
	if err := <-stopped; !errors.Is(err, context.Canceled) {
		t.Errorf("Stop() error = %v, want the context error while the sweep still runs", err)
	}
	release()
	waitFor(t, func() bool { return p.pool.Stat().TotalConns() == 0 })
}

func TestStopReturnsWhenItsContextEndsWhileAConnectionIsHeld(t *testing.T) {
	t.Parallel()

	p := newMigratedPlugin(t)
	conn, err := p.pool.Acquire(t.Context())
	if err != nil {
		t.Fatalf("Acquire() error = %v, want nil", err)
	}
	t.Cleanup(conn.Release)
	ctx, cancel := context.WithCancel(t.Context())
	stopped := make(chan error, 1)
	go func() { stopped <- p.Stop(ctx) }()

	cancel()

	waitFor(t, func() bool { return len(stopped) == 1 })
	if err := <-stopped; !errors.Is(err, context.Canceled) {
		t.Errorf("Stop() error = %v, want the context error while a connection is still held", err)
	}
	conn.Release()
	waitFor(t, func() bool { return p.pool.Stat().TotalConns() == 0 })
}
