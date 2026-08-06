// SPDX-License-Identifier: Elastic-2.0

package contact

import (
	"context"
	"errors"
	"strings"

	"github.com/google/uuid"

	"github.com/gopherium/alphone/internal/event"
)

// Store persists contacts and their channel identities.
type Store interface {
	Get(ctx context.Context, id uuid.UUID) (Contact, error)
	LookupIdentity(ctx context.Context, channel Channel, identifier string) (Identity, error)
	CreateContactWithIdentity(ctx context.Context, c Contact, identity Identity) error
	CreateContactWithIdentities(ctx context.Context, c Contact, identities []Identity) error
}

// Resolver finds or creates the contact owning a channel identity.
type Resolver struct {
	store  Store
	events Publisher
}

// Publisher announces domain events.
type Publisher interface {
	Publish(ctx context.Context, name event.Name, data map[string]any)
}

// Option configures a [Resolver].
type Option func(*Resolver)

// WithEvents announces contacts created by a channel to events.
func WithEvents(events Publisher) Option {
	return func(r *Resolver) { r.events = events }
}

// NewResolver returns a [Resolver] backed by store.
func NewResolver(store Store, options ...Option) *Resolver {
	r := &Resolver{store: store}
	for _, option := range options {
		option(r)
	}
	return r
}

// publish announces an event unless the resolver was built without a
// publisher.
func (r *Resolver) publish(ctx context.Context, name event.Name, data map[string]any) {
	if r.events == nil {
		return
	}
	r.events.Publish(ctx, name, data)
}

// Resolve returns the contact owning the identity for channel and
// identifier, creating both when unknown. A new contact is named after
// displayName, or after identifier when displayName is blank.
func (r *Resolver) Resolve(ctx context.Context, channel Channel, identifier, displayName string) (Contact, error) {
	normalizedChannel := normalizeChannel(channel)
	if normalizedChannel == "" {
		return Contact{}, ErrEmptyChannel
	}
	trimmedIdentifier := strings.TrimSpace(identifier)
	if trimmedIdentifier == "" {
		return Contact{}, ErrEmptyIdentifier
	}

	owner, err := r.owner(ctx, normalizedChannel, trimmedIdentifier)
	if err == nil {
		return owner, nil
	}
	if !errors.Is(err, ErrIdentityNotFound) {
		return Contact{}, err
	}
	return r.createOwner(ctx, normalizedChannel, trimmedIdentifier, displayName)
}

// createOwner stores a new contact owning the identity for channel and identifier.
func (r *Resolver) createOwner(ctx context.Context, channel Channel, identifier, displayName string) (Contact, error) {
	created, err := New(contactName(displayName, identifier))
	if err != nil {
		return Contact{}, err
	}
	identity, err := NewIdentity(created.ID, channel, identifier, displayName)
	if err != nil {
		return Contact{}, err
	}
	err = r.store.CreateContactWithIdentity(ctx, created, identity)
	if errors.Is(err, ErrIdentityExists) {
		return r.owner(ctx, channel, identifier)
	}
	if err != nil {
		return Contact{}, err
	}
	r.publish(ctx, event.ContactCreated, EventData(created))
	return created, nil
}

// owner returns the contact owning the identity for channel and identifier.
func (r *Resolver) owner(ctx context.Context, channel Channel, identifier string) (Contact, error) {
	identity, err := r.store.LookupIdentity(ctx, channel, identifier)
	if err != nil {
		return Contact{}, err
	}
	return r.store.Get(ctx, identity.ContactID)
}

// contactName returns a trimmed displayName, or identifier when displayName is blank.
func contactName(displayName, identifier string) string {
	if name := strings.TrimSpace(displayName); name != "" {
		return name
	}
	return identifier
}
