// SPDX-License-Identifier: Elastic-2.0

package contact

import (
	"context"
	"errors"
	"strings"

	"github.com/google/uuid"

	"github.com/gopherium/alphone/internal/event"
)

// Address is the channel and identifier a contact answers on, before it is stored.
type Address struct {
	Channel     Channel
	Identifier  string
	DisplayName string
}

// FindByIdentity returns the contact owning the identity, reporting whether one exists.
func (r *Resolver) FindByIdentity(
	ctx context.Context, channel Channel, identifier string,
) (Contact, bool, error) {
	normalizedChannel, normalizedIdentifier, err := identityKey(channel, identifier)
	if err != nil {
		return Contact{}, false, err
	}
	owner, err := r.owner(ctx, normalizedChannel, normalizedIdentifier)
	if errors.Is(err, ErrIdentityNotFound) {
		return Contact{}, false, nil
	}
	if err != nil {
		return Contact{}, false, err
	}
	return owner, true, nil
}

// CreateWithIdentities stores a contact named name owning every address,
// reporting whether it was created.
func (r *Resolver) CreateWithIdentities(
	ctx context.Context, name string, addresses []Address,
) (Contact, bool, error) {
	created, err := New(name)
	if err != nil {
		return Contact{}, false, err
	}
	identities, err := newIdentities(created.ID, addresses)
	if err != nil {
		return Contact{}, false, err
	}
	err = r.store.CreateContactWithIdentities(ctx, created, identities)
	var claimed IdentityExistsError
	if errors.As(err, &claimed) {
		owner, getErr := r.store.Get(ctx, claimed.OwnerID)
		return owner, false, getErr
	}
	if err != nil {
		return Contact{}, false, err
	}
	r.publish(ctx, event.Frame{Name: event.ContactCreated}, map[string]any{
		"id": created.ID.String(), "name": created.Name,
	})
	return created, true, nil
}

// identityKey returns the canonical channel and identifier an address is stored under.
func identityKey(channel Channel, identifier string) (Channel, string, error) {
	normalizedChannel := normalizeChannel(channel)
	if normalizedChannel == "" {
		return "", "", ErrEmptyChannel
	}
	normalizedIdentifier := normalizeIdentifier(normalizedChannel, strings.TrimSpace(identifier))
	if normalizedIdentifier == "" {
		return "", "", ErrEmptyIdentifier
	}
	return normalizedChannel, normalizedIdentifier, nil
}

// newIdentities returns the identities the addresses describe, all owned by contactID.
func newIdentities(contactID uuid.UUID, addresses []Address) ([]Identity, error) {
	identities := make([]Identity, 0, len(addresses))
	for _, address := range addresses {
		identity, err := NewIdentity(contactID, address.Channel, address.Identifier, address.DisplayName)
		if err != nil {
			return nil, err
		}
		identities = append(identities, identity)
	}
	return identities, nil
}
