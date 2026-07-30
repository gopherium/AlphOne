// SPDX-License-Identifier: Elastic-2.0

package webhook

import (
	"context"
	"log/slog"

	"github.com/gopherium/alphone/internal/event"
)

// DeliveryQueue matches subscriptions to an event and queues their
// deliveries.
type DeliveryQueue interface {
	ListSubscriptionsForEvent(ctx context.Context, name event.Name) ([]Subscription, error)
	EnqueueDelivery(ctx context.Context, d Delivery) error
}

// Dispatcher queues an event for every subscription that wants it.
type Dispatcher struct {
	queue  DeliveryQueue
	logger *slog.Logger
}

// NewDispatcher returns a [Dispatcher] queueing onto queue.
func NewDispatcher(queue DeliveryQueue, logger *slog.Logger) *Dispatcher {
	return &Dispatcher{queue: queue, logger: logger}
}

// Publish queues the named event for its subscribers, reporting failures to
// the log rather than to the caller, so a subscriber never breaks the work
// that produced the event.
func (d *Dispatcher) Publish(ctx context.Context, name event.Name, data map[string]any) {
	occurred, err := event.New(name, data)
	if err != nil {
		d.logger.ErrorContext(ctx, "refusing to publish", "event", string(name), "error", err)
		return
	}
	subs, err := d.queue.ListSubscriptionsForEvent(ctx, name)
	if err != nil {
		d.logger.ErrorContext(ctx, "listing webhook subscriptions", "event", string(name), "error", err)
		return
	}
	for _, sub := range subs {
		delivery, err := NewDelivery(sub, occurred)
		if err != nil {
			d.logger.ErrorContext(ctx, "building webhook delivery",
				"event", string(name), "subscription", sub.ID, "error", err)
			continue
		}
		if err := d.queue.EnqueueDelivery(ctx, delivery); err != nil {
			d.logger.ErrorContext(ctx, "queueing webhook delivery",
				"event", string(name), "subscription", sub.ID, "error", err)
		}
	}
}
