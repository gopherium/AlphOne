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

// ErrInvalidFieldText reports field text no live definition accepts.
var ErrInvalidFieldText = errors.New("sdk: invalid field text")

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

// AreaProvider is implemented by plugins holding their protected routes to one scope area.
type AreaProvider interface {
	Area() string
}

// RoleDeclaration is one role a plugin declares, or capabilities it adds to a role the host knows.
type RoleDeclaration struct {
	// Name is the role in its stored form.
	Name string
	// Capabilities names what the role holds, added to whatever it already carries.
	Capabilities []string
}

// RoleProvider is implemented by plugins declaring roles or widening the ones the host knows.
type RoleProvider interface {
	// Roles returns every role the plugin declares and every capability it adds to one.
	Roles() []RoleDeclaration
}

// GraphField is one runtime defined field a plugin serves over the graph.
type GraphField struct {
	// Entity is the GraphQL type the field hangs on, such as Contact.
	Entity string
	// Name is the field name a caller selects.
	Name string
	// Type is the GraphQL scalar the field answers with.
	Type string
}

// FieldSource reports the runtime defined fields a plugin serves.
type FieldSource interface {
	FieldsSnapshot(ctx context.Context) (uint64, []GraphField, error)
}

// ContactField is one live field a column may be mapped onto.
type ContactField struct {
	// Name is the field name a mapping stores.
	Name string
	// Label is the text an operator reads on screen.
	Label string
}

// FieldProvider serves live contact fields and writes their values from text.
type FieldProvider interface {
	// LiveContactFields lists every field a column may be mapped onto.
	LiveContactFields(ctx context.Context) ([]ContactField, error)
	// CheckContactFieldTexts reports a text no field accepts by wrapping ErrInvalidFieldText.
	CheckContactFieldTexts(ctx context.Context, values map[string]string) error
	// WriteContactFieldTexts stores the values the texts describe on one contact.
	WriteContactFieldTexts(ctx context.Context, contactID uuid.UUID, values map[string]string) error
}

// FieldConsumer receives the registered field providers.
type FieldConsumer interface {
	UseFieldProviders(providers []FieldProvider)
}

// CredentialProvider stores the calling tenant's sending identity for one channel.
type CredentialProvider interface {
	// Channel names the channel the credentials send on.
	Channel() Channel
	// SetChannelCredentials stores the calling tenant's identifier and secret, sealed at rest.
	SetChannelCredentials(ctx context.Context, identifier, secret string) error
	// ChannelIdentifier answers the calling tenant's configured identifier and whether one is set.
	ChannelIdentifier(ctx context.Context) (string, bool, error)
}

// CredentialConsumer receives the registered credential providers.
type CredentialConsumer interface {
	UseCredentialProviders(providers []CredentialProvider)
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
