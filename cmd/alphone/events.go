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
// broadcasts the frame to live listeners.
func (n nudgingPublisher) Publish(ctx context.Context, frame event.Frame, data map[string]any) {
	n.dispatcher.Publish(ctx, frame.Name, data)
	n.worker.Poke()
	n.hub.Broadcast(frame)
}

// pluginPublisher maps a plugin's untyped event name onto the core publisher.
type pluginPublisher struct {
	publisher nudgingPublisher
}

// Publish announces a plugin's event under its core name.
func (p pluginPublisher) Publish(ctx context.Context, name string, data map[string]any) {
	p.publisher.Publish(ctx, event.Frame{Name: event.Name(name)}, data)
}
