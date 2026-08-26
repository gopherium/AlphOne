// SPDX-License-Identifier: Elastic-2.0

package main

import (
	"context"
	"log/slog"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/gopherium/alphone/internal/event"
	"github.com/gopherium/alphone/internal/webhook"
	"github.com/gopherium/alphone/sdk"
)

type emptyQueue struct{}

// ListSubscriptionsForEvent returns no subscriptions.
func (emptyQueue) ListSubscriptionsForEvent(context.Context, event.Name) ([]webhook.Subscription, error) {
	return nil, nil
}

// EnqueueDelivery stores nothing.
func (emptyQueue) EnqueueDelivery(context.Context, webhook.Delivery) error {
	return nil
}

// ClaimDueDeliveries hands out nothing.
func (emptyQueue) ClaimDueDeliveries(context.Context, time.Time, time.Time, int) ([]webhook.ClaimedDelivery, error) {
	return nil, nil
}

// SettleDelivery records nothing.
func (emptyQueue) SettleDelivery(context.Context, uuid.UUID, string, time.Time, string) error {
	return nil
}

func TestPublishBroadcastsToTheLiveHub(t *testing.T) {
	t.Parallel()

	logger := slog.New(slog.DiscardHandler)
	hub := event.NewHub()
	assignee := uuid.Must(uuid.NewV7())
	addressed := hub.Subscribe(assignee, uuid.Nil)
	unaddressed := hub.Subscribe(uuid.Must(uuid.NewV7()), uuid.Nil)
	publisher := nudgingPublisher{
		dispatcher: webhook.NewDispatcher(emptyQueue{}, logger),
		worker:     webhook.NewWorker(emptyQueue{}, logger),
		hub:        hub,
	}

	publisher.Publish(
		t.Context(),
		event.Frame{Name: event.TaskCreated, Audience: assignee},
		map[string]any{"id": "abc"},
	)

	select {
	case got := <-addressed:
		if got != event.TaskCreated {
			t.Errorf("hub delivered %q, want %q", got, event.TaskCreated)
		}
	default:
		t.Error("hub delivered nothing, want the published name")
	}
	if len(unaddressed) != 0 {
		t.Errorf("a subscriber outside the audience buffered %d frames, want none", len(unaddressed))
	}
}

func TestAPluginEventStaysInsideTheCallersTenant(t *testing.T) {
	t.Parallel()

	logger := slog.New(slog.DiscardHandler)
	hub := event.NewHub()
	acme := uuid.Must(uuid.NewV7())
	near := hub.Subscribe(uuid.Must(uuid.NewV7()), acme)
	far := hub.Subscribe(uuid.Must(uuid.NewV7()), uuid.Must(uuid.NewV7()))
	publisher := pluginPublisher{publisher: nudgingPublisher{
		dispatcher: webhook.NewDispatcher(emptyQueue{}, logger),
		worker:     webhook.NewWorker(emptyQueue{}, logger),
		hub:        hub,
	}}

	publisher.Publish(sdk.WithTenant(t.Context(), acme),
		string(event.WhatsAppMessageReceived), map[string]any{"id": "abc"})

	select {
	case got := <-near:
		if got != event.WhatsAppMessageReceived {
			t.Errorf("hub delivered %q, want %q", got, event.WhatsAppMessageReceived)
		}
	default:
		t.Error("hub delivered nothing to the caller's tenant, want the published name")
	}
	if len(far) != 0 {
		t.Errorf("another tenant's subscriber buffered %d frames, want none", len(far))
	}
}

func TestAHeadlessPluginPublishLandsInTheDefaultTenant(t *testing.T) {
	t.Parallel()

	logger := slog.New(slog.DiscardHandler)
	hub := event.NewHub()
	elsewhere := hub.Subscribe(uuid.Must(uuid.NewV7()), sdk.DefaultTenantID)
	publisher := pluginPublisher{publisher: nudgingPublisher{
		dispatcher: webhook.NewDispatcher(emptyQueue{}, logger),
		worker:     webhook.NewWorker(emptyQueue{}, logger),
		hub:        hub,
	}}

	publisher.Publish(t.Context(), "whatsapp.message.received", map[string]any{"id": "abc"})

	select {
	case got := <-elsewhere:
		if got != event.Name("whatsapp.message.received") {
			t.Errorf("hub delivered %q, want the plugin event", got)
		}
	default:
		t.Error("hub delivered nothing to an unrelated subscriber, want a plugin event everyone sees")
	}
}
