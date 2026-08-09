// SPDX-License-Identifier: Elastic-2.0

// Package sdk defines the contract between the AlphOne core and its
// plugins. Together with graph, it is one of the two AlphOne packages a
// plugin may import.
//
// The contract is experimental until it is tagged v1.0.0; from then on
// it follows semantic versioning and the project deprecation policy.
package sdk

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"github.com/gopherium/pluginkit"
)

// ErrInvalidContact reports contact details the host refuses to store.
var ErrInvalidContact = errors.New("sdk: invalid contact details")

// Plugin is an independently addable unit of functionality with a
// managed lifecycle.
type Plugin = pluginkit.Plugin

// Migrator is implemented by plugins that own database schema, which
// the host migrates before starting any plugin.
type Migrator = pluginkit.Migrator

// RouteProvider is implemented by plugins that expose HTTP endpoints
// under their own namespace.
type RouteProvider = pluginkit.RouteProvider

// PublicPathProvider is implemented by plugins declaring session-exempt public paths.
type PublicPathProvider = pluginkit.PublicPathProvider

// Deps carries the host-provided dependencies a plugin receives at
// registration. By convention every plugin package exports
//
//	func Register(deps Deps) (*Plugin, error)
//
// which the host calls once per installed plugin.
type Deps struct {
	DatabaseURL string
	Resolver    ContactResolver
	Contacts    ContactDirectory
	Getenv      func(string) string
	Events      Publisher
}

// Publisher announces a plugin's own events to the host.
type Publisher interface {
	Publish(ctx context.Context, name string, data map[string]any)
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

// Identity is an address a contact answers on, stored in its channel's canonical form.
type Identity struct {
	Channel     Channel
	Identifier  string
	DisplayName string
}

// ContactDirectory looks contacts up by identity and creates contacts owning
// several identities at once.
type ContactDirectory interface {
	FindByIdentity(ctx context.Context, channel Channel, identifier string) (Contact, bool, error)
	CreateWithIdentities(ctx context.Context, name string, identities []Identity) (Contact, bool, error)
}
