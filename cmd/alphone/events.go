// SPDX-License-Identifier: Elastic-2.0

package main

import (
	"context"

	"github.com/gopherium/alphone/internal/event"
	"github.com/gopherium/alphone/internal/webhook"
)

// nudgingPublisher queues an event and wakes the delivery worker, so a
// subscriber hears about it without waiting for the next sweep.
type nudgingPublisher struct {
	dispatcher *webhook.Dispatcher
	worker     *webhook.Worker
}

// Publish queues the event for its subscribers and wakes the worker.
func (n nudgingPublisher) Publish(ctx context.Context, name event.Name, data map[string]any) {
	n.dispatcher.Publish(ctx, name, data)
	n.worker.Poke()
}

// pluginPublisher adapts the plugin contract's untyped event names onto the
// core publisher, which refuses any name the core does not publish.
type pluginPublisher struct {
	publisher nudgingPublisher
}

// Publish announces a plugin's event under its core name.
func (p pluginPublisher) Publish(ctx context.Context, name string, data map[string]any) {
	p.publisher.Publish(ctx, event.Name(name), data)
}
