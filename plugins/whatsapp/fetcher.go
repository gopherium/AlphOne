// SPDX-License-Identifier: Elastic-2.0

package whatsapp

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sync/atomic"
	"time"
)

// Media download tuning: sweep cadence and bounds, batch size, how long a
// webhook media id stays retryable, and the default stored blob cap.
const (
	mediaSweepInterval   = time.Minute
	mediaSweepTimeout    = 2 * time.Minute
	mediaSweepBatch      = 5
	mediaRetryWindow     = 7 * 24 * time.Hour
	mediaDownloadTimeout = time.Minute
	defaultMediaMaxBytes = 25 << 20
)

// mediaFetcherConfig parameterizes a mediaFetcher.
type mediaFetcherConfig struct {
	baseURL       string
	accessToken   string
	phoneNumberID string
	maxBytes      int64
	interval      time.Duration
	timeout       time.Duration
}

// mediaFetcher downloads pending media blobs from the Graph API until stopped.
type mediaFetcher struct {
	store         *store
	events        *broadcaster
	client        *http.Client
	baseURL       string
	accessToken   string
	phoneNumberID string
	maxBytes      int64
	interval      time.Duration
	timeout       time.Duration
	nudge         chan struct{}
	sweeps        atomic.Int64
	cancel        context.CancelFunc
	done          chan struct{}
}

// newMediaFetcher returns a mediaFetcher over cfg, applying defaults for its
// zero values.
func newMediaFetcher(s *store, events *broadcaster, cfg mediaFetcherConfig) *mediaFetcher {
	interval := cfg.interval
	if interval == 0 {
		interval = mediaSweepInterval
	}
	timeout := cfg.timeout
	if timeout == 0 {
		timeout = mediaSweepTimeout
	}
	maxBytes := cfg.maxBytes
	if maxBytes == 0 {
		maxBytes = defaultMediaMaxBytes
	}
	return &mediaFetcher{
		store:         s,
		events:        events,
		client:        &http.Client{Timeout: mediaDownloadTimeout},
		baseURL:       cfg.baseURL,
		accessToken:   cfg.accessToken,
		phoneNumberID: cfg.phoneNumberID,
		maxBytes:      maxBytes,
		interval:      interval,
		timeout:       timeout,
		nudge:         make(chan struct{}, 1),
	}
}

// Start launches the download loop in a goroutine. Call it once.
func (f *mediaFetcher) Start() {
	ctx, cancel := context.WithCancel(context.Background())
	f.cancel = cancel
	f.done = make(chan struct{})
	go func() {
		defer close(f.done)
		f.run(ctx)
	}()
}

// Stop cancels the download loop and waits for it to finish. Stopping a
// never started fetcher is not an error.
func (f *mediaFetcher) Stop() {
	if f.cancel == nil {
		return
	}
	f.cancel()
	<-f.done
}

// poke wakes the download loop for a fresh pending row without waiting for
// the next tick.
func (f *mediaFetcher) poke() {
	select {
	case f.nudge <- struct{}{}:
	default:
	}
}

// run sweeps once immediately, then on every tick or nudge until ctx ends.
func (f *mediaFetcher) run(ctx context.Context) {
	f.sweepOnce(ctx)
	ticker := time.NewTicker(f.interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			f.sweepOnce(ctx)
		case <-f.nudge:
			f.sweepOnce(ctx)
		}
	}
}

// sweepOnce downloads the media rows currently due, bounding the sweep to
// the fetcher's timeout.
func (f *mediaFetcher) sweepOnce(ctx context.Context) {
	defer f.sweeps.Add(1)
	sweepCtx, cancel := context.WithTimeout(ctx, f.timeout)
	defer cancel()
	pending, err := f.store.claimDueMedia(sweepCtx, time.Now().UTC(), mediaSweepBatch)
	if err != nil {
		return
	}
	for _, row := range pending {
		f.fetchOne(sweepCtx, row)
	}
}

// fetchOne resolves one pending media row to stored, rescheduled, or failed.
func (f *mediaFetcher) fetchOne(ctx context.Context, row pendingMediaRow) {
	if time.Since(row.CreatedAt) > mediaRetryWindow {
		f.settle(ctx, row, "expired")
		return
	}
	metadata, err := f.fetchMetadata(ctx, row.MediaID)
	if err != nil {
		f.reschedule(ctx, row, err.Error())
		return
	}
	if metadata.FileSize > f.maxBytes {
		f.settle(ctx, row, "too_large")
		return
	}
	data, err := f.download(ctx, metadata.URL)
	if err != nil {
		f.reschedule(ctx, row, err.Error())
		return
	}
	if int64(len(data)) > f.maxBytes {
		f.settle(ctx, row, "too_large")
		return
	}
	if !sha256Matches(row.SHA256, data) {
		f.reschedule(ctx, row, "sha256 mismatch")
		return
	}
	if err := f.store.markMediaStored(ctx, row.MessageID, data, metadata.MimeType, int64(len(data))); err != nil {
		return
	}
	f.events.broadcast(event{Conversation: row.ConversationID})
}

// settle retires the row with a terminal reason and announces the change.
func (f *mediaFetcher) settle(ctx context.Context, row pendingMediaRow, reason string) {
	if err := f.store.markMediaFailed(ctx, row.MessageID, reason); err != nil {
		return
	}
	f.events.broadcast(event{Conversation: row.ConversationID})
}

// reschedule defers the row's next download attempt with backoff.
func (f *mediaFetcher) reschedule(ctx context.Context, row pendingMediaRow, reason string) {
	_ = f.store.rescheduleMedia(ctx, row.MessageID, reason, time.Now().UTC().Add(backoff(row.Attempts)))
}

// backoff returns the retry delay after the given number of attempts, capped
// at one hour.
func backoff(attempts int) time.Duration {
	if attempts >= 7 {
		return time.Hour
	}
	return 30 * time.Second << attempts
}

type mediaMetadata struct {
	URL      string `json:"url"`
	MimeType string `json:"mime_type"`
	FileSize int64  `json:"file_size"`
}

// fetchMetadata asks the Graph API where a media asset can be downloaded.
func (f *mediaFetcher) fetchMetadata(ctx context.Context, mediaID string) (mediaMetadata, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet,
		f.baseURL+"/"+mediaID+"?phone_number_id="+f.phoneNumberID, nil)
	if err != nil {
		return mediaMetadata{}, fmt.Errorf("whatsapp: build metadata request: %w", err)
	}
	request.Header.Set("Authorization", "Bearer "+f.accessToken)
	response, err := f.client.Do(request)
	if err != nil {
		return mediaMetadata{}, fmt.Errorf("whatsapp: fetch media metadata: %w", err)
	}
	defer func() { _ = response.Body.Close() }()
	if response.StatusCode != http.StatusOK {
		return mediaMetadata{}, fmt.Errorf("whatsapp: media metadata status %d", response.StatusCode)
	}
	var metadata mediaMetadata
	if err := json.NewDecoder(response.Body).Decode(&metadata); err != nil {
		return mediaMetadata{}, fmt.Errorf("whatsapp: decode media metadata: %w", err)
	}
	return metadata, nil
}

// download fetches the media bytes from url, reading at most one byte over
// the fetcher's cap.
func (f *mediaFetcher) download(ctx context.Context, url string) ([]byte, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("whatsapp: build download request: %w", err)
	}
	request.Header.Set("Authorization", "Bearer "+f.accessToken)
	response, err := f.client.Do(request)
	if err != nil {
		return nil, fmt.Errorf("whatsapp: download media: %w", err)
	}
	defer func() { _ = response.Body.Close() }()
	if response.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("whatsapp: media download status %d", response.StatusCode)
	}
	data, err := io.ReadAll(io.LimitReader(response.Body, f.maxBytes+1))
	if err != nil {
		return nil, fmt.Errorf("whatsapp: download media: %w", err)
	}
	return data, nil
}

// sha256Matches reports whether data matches the base64 encoded hash,
// accepting hashes that do not decode as unverifiable.
func sha256Matches(encoded string, data []byte) bool {
	want, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil || len(want) != sha256.Size {
		return true
	}
	sum := sha256.Sum256(data)
	return bytes.Equal(sum[:], want)
}
