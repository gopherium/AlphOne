// SPDX-License-Identifier: Elastic-2.0

// Package webhook delivers domain events to subscribed HTTP endpoints.
package webhook

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"net/url"
	"slices"
	"time"

	"github.com/google/uuid"

	"github.com/gopherium/alphone/internal/event"
)

// ErrInvalidURL reports a delivery target that is not an absolute HTTP URL.
var ErrInvalidURL = errors.New("webhook: invalid url")

// ErrNoEvents reports a subscription naming no events.
var ErrNoEvents = errors.New("webhook: no events")

// ErrNotFound reports that no subscription exists for the requested ID.
var ErrNotFound = errors.New("webhook: not found")

// SecretPrefix identifies a webhook signing secret.
const SecretPrefix = "whsec_"

// secretBytes is how much entropy backs the part of a secret after
// [SecretPrefix].
const secretBytes = 32

// RetryWindow is how long a delivery is retried before it is given up on.
const RetryWindow = 24 * time.Hour

// baseBackoff is the delay before the first retry.
const baseBackoff = 30 * time.Second

// maxBackoff caps how long a retry is held back.
const maxBackoff = time.Hour

// defaultRandRead is the entropy source a secret is built from.
var defaultRandRead = rand.Read

// randRead draws the entropy a secret is built from.
var randRead = defaultRandRead

// Subscription is an endpoint receiving signed copies of named events.
type Subscription struct {
	ID        uuid.UUID
	UserID    uuid.UUID
	URL       string
	Events    []event.Name
	Secret    string
	CreatedAt time.Time
}

// NewSubscription returns a subscription delivering events to rawURL.
func NewSubscription(userID uuid.UUID, rawURL string, events []event.Name) (Subscription, error) {
	if err := validateURL(rawURL); err != nil {
		return Subscription{}, err
	}
	if len(events) == 0 {
		return Subscription{}, ErrNoEvents
	}
	for _, name := range events {
		if !name.Valid() {
			return Subscription{}, fmt.Errorf("%w: %q", event.ErrUnknownName, name)
		}
	}
	id, err := uuid.NewV7()
	if err != nil {
		return Subscription{}, fmt.Errorf("webhook: generate id: %w", err)
	}
	entropy := make([]byte, secretBytes)
	if _, err := randRead(entropy); err != nil {
		return Subscription{}, fmt.Errorf("webhook: draw secret: %w", err)
	}
	return Subscription{
		ID:        id,
		UserID:    userID,
		URL:       rawURL,
		Events:    slices.Clone(events),
		Secret:    SecretPrefix + base64.RawURLEncoding.EncodeToString(entropy),
		CreatedAt: time.Now().UTC(),
	}, nil
}

// Wants reports whether the subscription receives the named event.
func (s Subscription) Wants(name event.Name) bool {
	return slices.Contains(s.Events, name)
}

// Sign returns the signature header value for body under secret.
func Sign(secret string, body []byte) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(body)
	return "sha256=" + hex.EncodeToString(mac.Sum(nil))
}

// Backoff returns how long to wait before retrying after attempts failures.
func Backoff(attempts int) time.Duration {
	delay := baseBackoff << min(attempts-1, 16)
	if delay > maxBackoff || delay <= 0 {
		return maxBackoff
	}
	return delay
}

// validateURL reports whether raw is an absolute HTTP or HTTPS URL.
func validateURL(raw string) error {
	parsed, err := url.Parse(raw)
	if err != nil {
		return fmt.Errorf("%w: %q", ErrInvalidURL, raw)
	}
	if parsed.Host == "" || (parsed.Scheme != "http" && parsed.Scheme != "https") {
		return fmt.Errorf("%w: %q", ErrInvalidURL, raw)
	}
	return nil
}

// Delivery statuses.
const (
	StatusPending   = "pending"
	StatusDelivered = "delivered"
	StatusFailed    = "failed"
)

// Delivery is one queued attempt to hand an event to a subscriber.
type Delivery struct {
	ID             uuid.UUID
	SubscriptionID uuid.UUID
	EventID        uuid.UUID
	EventName      event.Name
	Payload        []byte
	Attempts       int
	DeliverAfter   time.Time
	Status         string
	LastError      string
	CreatedAt      time.Time
}

// NewDelivery returns a delivery of e to sub, due immediately.
func NewDelivery(sub Subscription, e event.Event) (Delivery, error) {
	payload, err := e.Payload()
	if err != nil {
		return Delivery{}, err
	}
	id, err := uuid.NewV7()
	if err != nil {
		return Delivery{}, fmt.Errorf("webhook: generate id: %w", err)
	}
	now := time.Now().UTC()
	return Delivery{
		ID:             id,
		SubscriptionID: sub.ID,
		EventID:        e.ID,
		EventName:      e.Name,
		Payload:        payload,
		DeliverAfter:   now,
		Status:         StatusPending,
		CreatedAt:      now,
	}, nil
}

// Exhausted reports whether a delivery has outlived its retry window at now.
func Exhausted(createdAt, now time.Time) bool {
	return now.Sub(createdAt) >= RetryWindow
}
