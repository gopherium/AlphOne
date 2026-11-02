// SPDX-License-Identifier: Elastic-2.0

package whatsapp

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sync/atomic"
	"time"

	"github.com/google/uuid"

	"github.com/gopherium/alphone/sdk"
)

// Media download tuning: sweep bounds, batch size, retry window, stored blob cap and redirect limit.
const (
	mediaSweepInterval   = time.Minute
	mediaSweepTimeout    = 2 * time.Minute
	mediaSweepBatch      = 5
	mediaRetryWindow     = 7 * 24 * time.Hour
	mediaDownloadTimeout = time.Minute
	defaultMediaMaxBytes = 25 << 20
	mediaRedirectLimit   = 10
)

// Media download errors.
var (
	errMediaScheme    = errors.New("whatsapp: media address leaves the scheme of the Graph URL")
	errMediaRedirects = fmt.Errorf("whatsapp: stopped after %d media redirects", mediaRedirectLimit)
	errMediaAddress   = errors.New("whatsapp: media address does not parse")
	errMediaMetadata  = errors.New("whatsapp: media metadata request failed")
	errMediaDownload  = errors.New("whatsapp: media download failed")
)

// requestFailure answers the fixed text a failed media request stores, otherwise when the failure is not one of ours.
func requestFailure(err, otherwise error) error {
	for _, ours := range []error{errMediaScheme, errMediaRedirects} {
		if errors.Is(err, ours) {
			return ours
		}
	}
	return otherwise
}

// mediaFetcherConfig parameterizes a mediaFetcher.
type mediaFetcherConfig struct {
	baseURL     string
	credentials func(ctx context.Context, number string) (credentials, error)
	records     func(ctx context.Context, tenantID uuid.UUID) (bool, error)
	maxBytes    int64
	interval    time.Duration
	timeout     time.Duration
}

// mediaFetcher downloads pending media blobs from the Graph API until stopped.
type mediaFetcher struct {
	store       *store
	events      *broadcaster
	client      *http.Client
	baseURL     string
	credentials func(ctx context.Context, number string) (credentials, error)
	records     func(ctx context.Context, tenantID uuid.UUID) (bool, error)
	maxBytes    int64
	interval    time.Duration
	timeout     time.Duration
	nudge       chan struct{}
	sweeps      atomic.Int64
	cancel      context.CancelFunc
	done        chan struct{}
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
	records := cfg.records
	if records == nil {
		records = func(context.Context, uuid.UUID) (bool, error) { return true, nil }
	}
	f := &mediaFetcher{
		store:       s,
		events:      events,
		client:      newOutboundClient(mediaDownloadTimeout),
		baseURL:     cfg.baseURL,
		credentials: cfg.credentials,
		records:     records,
		maxBytes:    maxBytes,
		interval:    interval,
		timeout:     timeout,
		nudge:       make(chan struct{}, 1),
	}
	f.client.CheckRedirect = f.checkRedirect
	return f
}

// checkRedirect keeps a media request on the scheme of the Graph URL and stops it where Go's default does.
func (f *mediaFetcher) checkRedirect(request *http.Request, via []*http.Request) error {
	if len(via) >= mediaRedirectLimit {
		return errMediaRedirects
	}
	if !f.onGraphScheme(request.URL) {
		return errMediaScheme
	}
	return nil
}

// onGraphScheme reports whether an address carries the scheme of the Graph URL, no scheme never matching.
func (f *mediaFetcher) onGraphScheme(address *url.URL) bool {
	graph, err := url.Parse(f.baseURL)
	return err == nil && address.Scheme != "" && address.Scheme == graph.Scheme
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

// Stop cancels the download loop and waits for it to finish, returning the error of ctx when ctx ends first.
func (f *mediaFetcher) Stop(ctx context.Context) error {
	if f.cancel == nil {
		return nil
	}
	f.cancel()
	select {
	case <-f.done:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
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
	now := time.Now().UTC()
	closed, err := f.closedTenants(sweepCtx, now)
	if err != nil {
		return
	}
	pending, err := f.store.claimDueMedia(sweepCtx, now, mediaSweepBatch, closed)
	if err != nil {
		return
	}
	for _, row := range pending {
		f.fetchOne(sdk.WithTenant(sweepCtx, row.TenantID), row)
	}
}

// closedTenants returns the tenants holding media due at now that record no traffic, a failing gate counting as closed.
func (f *mediaFetcher) closedTenants(ctx context.Context, now time.Time) ([]uuid.UUID, error) {
	due, err := f.store.tenantsWithDueMedia(ctx, now)
	if err != nil {
		return nil, err
	}
	closed := make([]uuid.UUID, 0, len(due))
	for _, tenantID := range due {
		if recording, err := f.records(sdk.WithTenant(ctx, tenantID), tenantID); err != nil || !recording {
			closed = append(closed, tenantID)
		}
	}
	return closed, nil
}

// fetchOne resolves one pending media row to stored, rescheduled, or failed.
func (f *mediaFetcher) fetchOne(ctx context.Context, row pendingMediaRow) {
	if time.Since(row.CreatedAt) > mediaRetryWindow {
		f.settle(ctx, row, "expired")
		return
	}
	creds, err := f.credentials(ctx, row.PhoneNumberID)
	if err != nil {
		f.reschedule(ctx, row, err.Error())
		return
	}
	metadata, err := f.fetchMetadata(ctx, creds, row.MediaID)
	if err != nil {
		f.reschedule(ctx, row, err.Error())
		return
	}
	if metadata.FileSize > f.maxBytes {
		f.settle(ctx, row, "too_large")
		return
	}
	data, err := f.download(ctx, creds, metadata.URL)
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
	f.events.broadcast(event{Conversation: row.ConversationID, Tenant: row.TenantID})
}

// settle retires the row with a terminal reason and announces the change.
func (f *mediaFetcher) settle(ctx context.Context, row pendingMediaRow, reason string) {
	if err := f.store.markMediaFailed(ctx, row.MessageID, reason); err != nil {
		return
	}
	f.events.broadcast(event{Conversation: row.ConversationID, Tenant: row.TenantID})
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

// fetchMetadata asks the Graph API as creds where a media asset can be downloaded.
func (f *mediaFetcher) fetchMetadata(ctx context.Context, creds credentials, mediaID string) (mediaMetadata, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet,
		f.baseURL+"/"+url.PathEscape(mediaID)+"?phone_number_id="+url.QueryEscape(creds.phoneNumberID), nil)
	if err != nil {
		return mediaMetadata{}, errMediaAddress
	}
	request.Header.Set("Authorization", "Bearer "+creds.accessToken)
	response, err := f.client.Do(request)
	if err != nil {
		return mediaMetadata{}, requestFailure(err, errMediaMetadata)
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

// download fetches media as creds from an address on the Graph URL's scheme, reading one byte past the cap at most.
func (f *mediaFetcher) download(ctx context.Context, creds credentials, address string) ([]byte, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, address, nil)
	if err != nil {
		return nil, errMediaAddress
	}
	if !f.onGraphScheme(request.URL) {
		return nil, errMediaScheme
	}
	request.Header.Set("Authorization", "Bearer "+creds.accessToken)
	response, err := f.client.Do(request)
	if err != nil {
		return nil, requestFailure(err, errMediaDownload)
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
