// SPDX-License-Identifier: Elastic-2.0

package whatsapp

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
)

type graphStub struct {
	mu           sync.Mutex
	server       *httptest.Server
	binary       []byte
	mimeType     string
	fileSize     int64
	metadataCode int
	metadataBody string
	binaryCode   int
	urlOverride  string
	truncate     bool
	authHeaders  []string
	metadataHits int
	binaryHits   int
}

func newGraphStub(t *testing.T, binary []byte) *graphStub {
	t.Helper()
	s := &graphStub{
		binary:       binary,
		mimeType:     "image/jpeg",
		fileSize:     int64(len(binary)),
		metadataCode: http.StatusOK,
		binaryCode:   http.StatusOK,
	}
	s.server = httptest.NewServer(http.HandlerFunc(s.handle))
	t.Cleanup(s.server.Close)
	return s
}

func (s *graphStub) handle(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.authHeaders = append(s.authHeaders, r.Header.Get("Authorization"))
	if strings.HasPrefix(r.URL.Path, "/binary") {
		s.binaryHits++
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

func (s *graphStub) hits() (int, int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.metadataHits, s.binaryHits
}

func (s *graphStub) headers() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]string(nil), s.authHeaders...)
}

func shaOf(data []byte) string {
	sum := sha256.Sum256(data)
	return base64.StdEncoding.EncodeToString(sum[:])
}

func newTestFetcher(t *testing.T, p *Plugin, baseURL string, maxBytes int64) *mediaFetcher {
	t.Helper()
	return newMediaFetcher(p.store, p.events, mediaFetcherConfig{
		baseURL:       baseURL,
		accessToken:   "test-token",
		phoneNumberID: "PN1",
		maxBytes:      maxBytes,
		interval:      time.Hour,
		timeout:       time.Minute,
	})
}

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
	subscription := p.events.subscribe()
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
			reason:    "request",
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
			reason:    "request",
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
			subscription := p.events.subscribe()
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
			pending, err := p.store.claimDueMedia(t.Context(), time.Now().UTC(), 10)
			if err != nil {
				t.Fatalf("claimDueMedia() error = %v, want nil", err)
			}
			if len(pending) != 0 {
				t.Errorf("row still due immediately after a retry, want a deferred next attempt")
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
	subscription := p.events.subscribe()
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
	subscription := p.events.subscribe()
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
			baseURL: stub.server.URL,
		})

		f.fetchOne(t.Context(), row)
	})

	t.Run("storing", func(t *testing.T) {
		t.Parallel()
		stub := newGraphStub(t, binary)
		f := newMediaFetcher(&store{pool: newUnreachablePool(t)}, newBroadcaster(), mediaFetcherConfig{
			baseURL: stub.server.URL,
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
	defer f.Stop()

	waitFor(t, func() bool { return mediaStatus(p, messageID) == "stored" })
}

func TestFetcherNudgeWakesTheLoop(t *testing.T) {
	t.Parallel()

	p := newMigratedPlugin(t)
	binary := []byte("media-bytes")
	stub := newGraphStub(t, binary)
	f := newTestFetcher(t, p, stub.server.URL, 1<<20)
	f.Start()
	defer f.Stop()
	waitFor(t, func() bool { return f.sweeps.Load() >= 1 })
	messageID := seedPendingImage(t, p, "wamid.nudged", shaOf(binary))

	f.poke()

	waitFor(t, func() bool { return mediaStatus(p, messageID) == "stored" })
}

func TestFetcherTickerSweeps(t *testing.T) {
	t.Parallel()

	p := newMigratedPlugin(t)
	binary := []byte("media-bytes")
	stub := newGraphStub(t, binary)
	f := newMediaFetcher(p.store, p.events, mediaFetcherConfig{
		baseURL:       stub.server.URL,
		accessToken:   "test-token",
		phoneNumberID: "PN1",
		maxBytes:      1 << 20,
		interval:      25 * time.Millisecond,
		timeout:       time.Minute,
	})
	f.Start()
	defer f.Stop()
	waitFor(t, func() bool { return f.sweeps.Load() >= 1 })
	messageID := seedPendingImage(t, p, "wamid.ticked", shaOf(binary))

	waitFor(t, func() bool { return mediaStatus(p, messageID) == "stored" })
}

func TestFetcherStopWithoutStart(t *testing.T) {
	t.Parallel()

	f := newMediaFetcher(&store{}, newBroadcaster(), mediaFetcherConfig{})

	f.Stop()
}
