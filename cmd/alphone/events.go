// SPDX-License-Identifier: Elastic-2.0

package main

import (
	"context"

	"github.com/gopherium/alphone/internal/event"
	"github.com/gopherium/alphone/internal/webhook"
)

// pluginPublisher adapts the plugin contract's untyped event names onto the
// core dispatcher, which refuses any name the core does not publish.
type pluginPublisher struct {
	dispatcher *webhook.Dispatcher
}

// Publish announces a plugin's event under its core name.
func (p pluginPublisher) Publish(ctx context.Context, name string, data map[string]any) {
	p.dispatcher.Publish(ctx, event.Name(name), data)
}
