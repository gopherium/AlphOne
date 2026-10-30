// SPDX-License-Identifier: Elastic-2.0

// Package email is the email channel plugin.
package email

import (
	"context"

	"github.com/gopherium/alphone/sdk"
)

// Plugin is the email channel, registered and idle.
type Plugin struct{}

// Register builds the email [Plugin] from the host-provided deps.
func Register(_ sdk.Deps) (*Plugin, error) {
	return &Plugin{}, nil
}

// ID reports the plugin identifier.
func (p *Plugin) ID() string {
	return "email"
}

// Start reports the plugin ready.
func (p *Plugin) Start(_ context.Context) error {
	return nil
}

// Stop reports the plugin released.
func (p *Plugin) Stop(_ context.Context) error {
	return nil
}
