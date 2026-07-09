// SPDX-License-Identifier: Elastic-2.0

package contact

import (
	"context"
	"errors"
	"strings"

	"github.com/google/uuid"
)

// Store persists contacts and their channel identities.
type Store interface {
	Get(ctx context.Context, id uuid.UUID) (Contact, error)
	LookupIdentity(ctx context.Context, channel Channel, identifier string) (Identity, error)
	CreateContactWithIdentity(ctx context.Context, c Contact, identity Identity) error
}

// Resolver finds or creates the contact owning a channel identity.
type Resolver struct {
	store Store
}

// NewResolver returns a [Resolver] backed by store.
func NewResolver(store Store) *Resolver {
	return &Resolver{store: store}
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

	created, err := New(contactName(displayName, trimmedIdentifier))
	if err != nil {
		return Contact{}, err
	}
	identity, err := NewIdentity(created.ID, normalizedChannel, trimmedIdentifier, displayName)
	if err != nil {
		return Contact{}, err
	}
	err = r.store.CreateContactWithIdentity(ctx, created, identity)
	if errors.Is(err, ErrIdentityExists) {
		return r.owner(ctx, normalizedChannel, trimmedIdentifier)
	}
	if err != nil {
		return Contact{}, err
	}
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
