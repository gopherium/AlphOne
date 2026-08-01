// SPDX-License-Identifier: Elastic-2.0

package main

import (
	"context"

	"github.com/gopherium/alphone/internal/event"
	"github.com/gopherium/alphone/internal/webhook"
)

// nudgingPublisher queues an event, wakes the delivery worker, and announces
// the name on the live hub.
type nudgingPublisher struct {
	dispatcher *webhook.Dispatcher
	worker     *webhook.Worker
	hub        *event.Hub
}

// Publish queues the event for its subscribers, wakes the worker, and
// broadcasts the name to live listeners.
func (n nudgingPublisher) Publish(ctx context.Context, name event.Name, data map[string]any) {
	n.dispatcher.Publish(ctx, name, data)
	n.worker.Poke()
	n.hub.Broadcast(name)
}

// pluginPublisher maps a plugin's untyped event name onto the core publisher.
type pluginPublisher struct {
	publisher nudgingPublisher
}

// Publish announces a plugin's event under its core name.
func (p pluginPublisher) Publish(ctx context.Context, name string, data map[string]any) {
	p.publisher.Publish(ctx, event.Name(name), data)
}
