// SPDX-License-Identifier: AGPL-3.0-or-later

// Package plugin hosts AlphOne's compile-time plugins.
package plugin

import (
	"context"
	"errors"
	"fmt"
	"net/http"

	"github.com/gopherium/alphone/sdk"
)

// Host starts and stops a fixed set of plugins.
type Host struct {
	plugins []sdk.Plugin
}

// NewHost returns a [Host] managing plugins. It panics when two
// plugins share an ID.
func NewHost(plugins ...sdk.Plugin) *Host {
	seen := make(map[string]struct{}, len(plugins))
	for _, p := range plugins {
		if _, ok := seen[p.ID()]; ok {
			panic(fmt.Sprintf("plugin: duplicate id %q", p.ID()))
		}
		seen[p.ID()] = struct{}{}
	}
	return &Host{plugins: plugins}
}

// Start migrates every [sdk.Migrator] plugin, then starts every plugin
// in registration order. When a start fails, the already-started
// plugins are stopped in reverse order and the failure is returned.
func (h *Host) Start(ctx context.Context) error {
	for _, p := range h.plugins {
		migrator, ok := p.(sdk.Migrator)
		if !ok {
			continue
		}
		if err := safeCall(ctx, p.ID(), "migrate", migrator.Migrate); err != nil {
			return err
		}
	}
	for i, p := range h.plugins {
		if err := safeCall(ctx, p.ID(), "start", p.Start); err != nil {
			return errors.Join(err, h.stopDownFrom(ctx, i-1))
		}
	}
	return nil
}

// Routes returns the HTTP handler of every [sdk.RouteProvider] plugin,
// keyed by plugin ID.
func (h *Host) Routes() map[string]http.Handler {
	routes := make(map[string]http.Handler)
	for _, p := range h.plugins {
		if provider, ok := p.(sdk.RouteProvider); ok {
			routes[p.ID()] = provider.Routes()
		}
	}
	return routes
}

// Stop stops every plugin in reverse registration order, continuing
// past failures and returning them joined.
func (h *Host) Stop(ctx context.Context) error {
	return h.stopDownFrom(ctx, len(h.plugins)-1)
}

// stopDownFrom stops plugins from index down to zero in reverse order, collecting and joining any errors.
func (h *Host) stopDownFrom(ctx context.Context, index int) error {
	var errs []error
	for i := index; i >= 0; i-- {
		if err := safeCall(ctx, h.plugins[i].ID(), "stop", h.plugins[i].Stop); err != nil {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}

// safeCall runs fn, wrapping any returned error and converting any panic into an error tagged with the plugin id and operation.
func safeCall(ctx context.Context, id, operation string, fn func(context.Context) error) (err error) {
	defer func() {
		if recovered := recover(); recovered != nil {
			err = fmt.Errorf("plugin: %s %s panicked: %v", id, operation, recovered)
		}
	}()
	if err := fn(ctx); err != nil {
		return fmt.Errorf("plugin: %s %s: %w", id, operation, err)
	}
	return nil
}
