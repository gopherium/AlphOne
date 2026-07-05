// SPDX-License-Identifier: AGPL-3.0-or-later

// Package plugin hosts AlphOne's compile-time plugins.
package plugin

import (
	"context"
	"errors"
	"fmt"
)

// Plugin is an independently addable unit of functionality with a
// managed lifecycle.
type Plugin interface {
	ID() string
	Start(ctx context.Context) error
	Stop(ctx context.Context) error
}

// Host starts and stops a fixed set of plugins.
type Host struct {
	plugins []Plugin
}

// NewHost returns a [Host] managing plugins, rejecting duplicate IDs.
func NewHost(plugins ...Plugin) (*Host, error) {
	seen := make(map[string]struct{}, len(plugins))
	for _, p := range plugins {
		if _, ok := seen[p.ID()]; ok {
			return nil, fmt.Errorf("plugin: duplicate id %q", p.ID())
		}
		seen[p.ID()] = struct{}{}
	}
	return &Host{plugins: plugins}, nil
}

// Start starts every plugin in registration order. When one fails, the
// already-started plugins are stopped in reverse order and the failure
// is returned.
func (h *Host) Start(ctx context.Context) error {
	for i, p := range h.plugins {
		if err := safeCall(ctx, p.ID(), "start", p.Start); err != nil {
			return errors.Join(err, h.stopDownFrom(ctx, i-1))
		}
	}
	return nil
}

// Stop stops every plugin in reverse registration order, continuing
// past failures and returning them joined.
func (h *Host) Stop(ctx context.Context) error {
	return h.stopDownFrom(ctx, len(h.plugins)-1)
}

func (h *Host) stopDownFrom(ctx context.Context, index int) error {
	var errs []error
	for i := index; i >= 0; i-- {
		if err := safeCall(ctx, h.plugins[i].ID(), "stop", h.plugins[i].Stop); err != nil {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}

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
