// SPDX-License-Identifier: Elastic-2.0

package main

import (
	"context"
	"log/slog"

	"github.com/gopherium/alphone/internal/event"
	"github.com/gopherium/alphone/internal/webhook"
	"github.com/gopherium/alphone/sdk"
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
	logger    *slog.Logger
}

// Publish announces a declared plugin event under its core name in the caller's tenant, dropping an undeclared one.
func (p pluginPublisher) Publish(ctx context.Context, name string, data map[string]any) {
	if !event.Name(name).Valid() {
		p.logger.ErrorContext(ctx, "a plugin published an event it never declared", "event", name)
		return
	}
	p.publisher.Publish(ctx, event.Frame{
		Name:   event.Name(name),
		Tenant: sdk.TenantOrDefault(ctx),
	}, data)
}

// declareEvents records the event names every registered plugin publishes, refusing a clash.
func declareEvents(registered []sdk.Plugin) error {
	for _, plugin := range registered {
		if provider, ok := plugin.(sdk.EventProvider); ok {
			if err := event.Declare(plugin.ID(), provider.Events()...); err != nil {
				return err
			}
		}
	}
	return nil
}
