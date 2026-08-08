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
	addressed := hub.Subscribe(assignee)
	unaddressed := hub.Subscribe(uuid.Must(uuid.NewV7()))
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

func TestPluginPublishReachesEverySubscriber(t *testing.T) {
	t.Parallel()

	logger := slog.New(slog.DiscardHandler)
	hub := event.NewHub()
	elsewhere := hub.Subscribe(uuid.Must(uuid.NewV7()))
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
