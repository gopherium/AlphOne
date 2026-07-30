// SPDX-License-Identifier: Elastic-2.0

package webhook_test

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/gopherium/alphone/internal/event"
	"github.com/gopherium/alphone/internal/webhook"
)

func TestNewSubscriptionMintsASigningSecret(t *testing.T) {
	t.Parallel()

	owner := uuid.Must(uuid.NewV7())

	created, err := webhook.NewSubscription(owner, "https://example.com/hook", []event.Name{event.TaskCreated})

	if err != nil {
		t.Fatalf("NewSubscription() error = %v, want nil", err)
	}
	if !strings.HasPrefix(created.Secret, webhook.SecretPrefix) {
		t.Errorf("Secret = %q, want prefix %q", created.Secret, webhook.SecretPrefix)
	}
	if created.ID == uuid.Nil {
		t.Error("ID = zero, want a generated identifier")
	}
	if created.UserID != owner {
		t.Errorf("UserID = %v, want %v", created.UserID, owner)
	}
	if created.CreatedAt.IsZero() {
		t.Error("CreatedAt = zero, want the creation time")
	}
}

func TestNewSubscriptionRejectsUnusableURLs(t *testing.T) {
	t.Parallel()

	for name, raw := range map[string]string{
		"empty":         "",
		"relative":      "/hook",
		"no host":       "https://",
		"wrong scheme":  "ftp://example.com/hook",
		"not a url":     "://nonsense",
		"missing space": "https://exa mple.com/hook",
	} {
		_, err := webhook.NewSubscription(uuid.Nil, raw, []event.Name{event.TaskCreated})

		if !errors.Is(err, webhook.ErrInvalidURL) {
			t.Errorf("%s: error = %v, want %v", name, err, webhook.ErrInvalidURL)
		}
	}
}

func TestNewSubscriptionRequiresAtLeastOneKnownEvent(t *testing.T) {
	t.Parallel()

	if _, err := webhook.NewSubscription(uuid.Nil, "https://example.com/h", nil); !errors.Is(err, webhook.ErrNoEvents) {
		t.Errorf("no events: error = %v, want %v", err, webhook.ErrNoEvents)
	}

	_, err := webhook.NewSubscription(uuid.Nil, "https://example.com/h", []event.Name{"task.deleted"})

	if !errors.Is(err, event.ErrUnknownName) {
		t.Errorf("unknown event: error = %v, want %v", err, event.ErrUnknownName)
	}
}

func TestWantsMatchesOnlySubscribedEvents(t *testing.T) {
	t.Parallel()

	created, err := webhook.NewSubscription(
		uuid.Nil, "https://example.com/h", []event.Name{event.TaskCreated, event.ContactCreated},
	)
	if err != nil {
		t.Fatalf("NewSubscription() error = %v, want nil", err)
	}

	if !created.Wants(event.TaskCreated) {
		t.Error("Wants(task.created) = false, want true")
	}
	if created.Wants(event.TaskCompleted) {
		t.Error("Wants(task.completed) = true, want false")
	}
}

func TestSubscriptionEventsCannotBeMutatedByItsCaller(t *testing.T) {
	t.Parallel()

	events := []event.Name{event.TaskCreated}

	created, err := webhook.NewSubscription(uuid.Nil, "https://example.com/h", events)
	if err != nil {
		t.Fatalf("NewSubscription() error = %v, want nil", err)
	}
	events[0] = event.TaskCompleted

	if created.Wants(event.TaskCompleted) {
		t.Error("a caller rewrote the subscription's events after it was built")
	}
	if !created.Wants(event.TaskCreated) {
		t.Error("a caller removed the subscription's original event")
	}
}

func TestSignProducesAVerifiableSignature(t *testing.T) {
	t.Parallel()

	body := []byte(`{"event":"task.created"}`)

	signature := webhook.Sign("whsec_example", body)

	mac := hmac.New(sha256.New, []byte("whsec_example"))
	mac.Write(body)
	want := "sha256=" + hex.EncodeToString(mac.Sum(nil))
	if signature != want {
		t.Errorf("Sign() = %q, want %q", signature, want)
	}
}

func TestSignDependsOnBothSecretAndBody(t *testing.T) {
	t.Parallel()

	body := []byte(`{"event":"task.created"}`)

	if webhook.Sign("whsec_a", body) == webhook.Sign("whsec_b", body) {
		t.Error("signatures match across different secrets")
	}
	if webhook.Sign("whsec_a", body) == webhook.Sign("whsec_a", []byte(`{"event":"task.completed"}`)) {
		t.Error("signatures match across different bodies")
	}
}

func TestBackoffGrowsAndIsCapped(t *testing.T) {
	t.Parallel()

	first := webhook.Backoff(1)
	second := webhook.Backoff(2)

	if first <= 0 {
		t.Errorf("Backoff(1) = %v, want a positive delay", first)
	}
	if second <= first {
		t.Errorf("Backoff(2) = %v, want more than Backoff(1) = %v", second, first)
	}
	if capped := webhook.Backoff(99); capped > time.Hour {
		t.Errorf("Backoff(99) = %v, want it capped at an hour or less", capped)
	}
}

func TestNewDeliveryQueuesTheSignedPayload(t *testing.T) {
	t.Parallel()

	sub, err := webhook.NewSubscription(
		uuid.Must(uuid.NewV7()), "https://example.com/h", []event.Name{event.TaskCreated},
	)
	if err != nil {
		t.Fatalf("NewSubscription() error = %v, want nil", err)
	}
	occurred, err := event.New(event.TaskCreated, map[string]any{"title": "Call Maria"})
	if err != nil {
		t.Fatalf("event.New() error = %v, want nil", err)
	}

	delivery, err := webhook.NewDelivery(sub, occurred)

	if err != nil {
		t.Fatalf("NewDelivery() error = %v, want nil", err)
	}
	if delivery.SubscriptionID != sub.ID {
		t.Errorf("SubscriptionID = %v, want %v", delivery.SubscriptionID, sub.ID)
	}
	if delivery.EventID != occurred.ID {
		t.Errorf("EventID = %v, want %v", delivery.EventID, occurred.ID)
	}
	if delivery.EventName != event.TaskCreated {
		t.Errorf("EventName = %q, want %q", delivery.EventName, event.TaskCreated)
	}
	if delivery.Status != webhook.StatusPending {
		t.Errorf("Status = %q, want %q", delivery.Status, webhook.StatusPending)
	}
	if delivery.Attempts != 0 {
		t.Errorf("Attempts = %d, want 0", delivery.Attempts)
	}
	if !strings.Contains(string(delivery.Payload), "Call Maria") {
		t.Errorf("Payload = %s, want the event data", delivery.Payload)
	}
	if delivery.DeliverAfter.IsZero() {
		t.Error("DeliverAfter = zero, want it due immediately")
	}
}

func TestNewDeliveryReportsAnUnencodableEvent(t *testing.T) {
	t.Parallel()

	sub, err := webhook.NewSubscription(uuid.Nil, "https://example.com/h", []event.Name{event.TaskCreated})
	if err != nil {
		t.Fatalf("NewSubscription() error = %v, want nil", err)
	}
	occurred, err := event.New(event.TaskCreated, map[string]any{"ch": make(chan int)})
	if err != nil {
		t.Fatalf("event.New() error = %v, want nil", err)
	}

	if _, err := webhook.NewDelivery(sub, occurred); err == nil {
		t.Error("NewDelivery() error = nil, want the encoding failure reported")
	}
}

func TestExhaustedReportsTheAttemptBudget(t *testing.T) {
	t.Parallel()

	if webhook.Exhausted(webhook.MaxAttempts - 1) {
		t.Error("Exhausted() true below the budget, want false")
	}
	if !webhook.Exhausted(webhook.MaxAttempts) {
		t.Error("Exhausted() false at the budget, want true")
	}
}
