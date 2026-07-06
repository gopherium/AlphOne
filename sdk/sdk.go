// SPDX-License-Identifier: AGPL-3.0-or-later

// Package sdk defines the contract between the AlphOne core and its
// plugins. It is the only AlphOne package a plugin may import.
//
// The contract is experimental until it is tagged v1.0.0; from then on
// it follows semantic versioning and the project deprecation policy.
package sdk

import (
	"context"
	"net/http"

	"github.com/google/uuid"
)

// Plugin is an independently addable unit of functionality with a
// managed lifecycle.
type Plugin interface {
	ID() string
	Start(ctx context.Context) error
	Stop(ctx context.Context) error
}

// Migrator is implemented by plugins that own database schema, which
// the host migrates before starting any plugin.
type Migrator interface {
	Migrate(ctx context.Context) error
}

// RouteProvider is implemented by plugins that expose HTTP endpoints
// under their own namespace.
type RouteProvider interface {
	Routes() http.Handler
}

// Channel names a communication medium, such as "whatsapp" or "email".
type Channel string

// Contact is the person a channel identity resolves to.
type Contact struct {
	ID   uuid.UUID
	Name string
}

// ContactResolver finds or creates the contact owning a channel
// identity.
type ContactResolver interface {
	Resolve(ctx context.Context, channel Channel, identifier, displayName string) (Contact, error)
}
