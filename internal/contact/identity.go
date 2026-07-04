// SPDX-License-Identifier: AGPL-3.0-or-later

package contact

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
)

// ErrNilContactID reports that an identity was created without an owning contact.
var ErrNilContactID = errors.New("contact: nil contact id")

// ErrEmptyChannel reports that an identity channel is empty or only whitespace.
var ErrEmptyChannel = errors.New("contact: empty channel")

// ErrEmptyIdentifier reports that an identity identifier is empty or only whitespace.
var ErrEmptyIdentifier = errors.New("contact: empty identifier")

// ErrIdentityNotFound reports that no identity exists for a channel and identifier.
var ErrIdentityNotFound = errors.New("contact: identity not found")

// ErrIdentityExists reports that a channel and identifier pair is already claimed.
var ErrIdentityExists = errors.New("contact: identity already exists")

// Channel names the communication medium of an identity, such as
// "whatsapp" or "email". Valid values are defined by channel plugins,
// never enumerated by the core.
type Channel string

// Identity is a per-channel address of a Contact. Identifier is opaque
// to the core and stored exactly as its channel delivered it.
type Identity struct {
	ID          uuid.UUID
	ContactID   uuid.UUID
	Channel     Channel
	Identifier  string
	DisplayName string
	CreatedAt   time.Time
}

// NewIdentity returns an Identity owned by contactID. The channel is
// trimmed and lowercased; identifier and display name are trimmed but
// otherwise stored verbatim.
func NewIdentity(contactID uuid.UUID, channel Channel, identifier, displayName string) (Identity, error) {
	if contactID == uuid.Nil {
		return Identity{}, ErrNilContactID
	}
	normalizedChannel := Channel(strings.ToLower(strings.TrimSpace(string(channel))))
	if normalizedChannel == "" {
		return Identity{}, ErrEmptyChannel
	}
	trimmedIdentifier := strings.TrimSpace(identifier)
	if trimmedIdentifier == "" {
		return Identity{}, ErrEmptyIdentifier
	}
	id, err := uuid.NewV7()
	if err != nil {
		return Identity{}, fmt.Errorf("contact: generate identity id: %w", err)
	}
	return Identity{
		ID:          id,
		ContactID:   contactID,
		Channel:     normalizedChannel,
		Identifier:  trimmedIdentifier,
		DisplayName: strings.TrimSpace(displayName),
		CreatedAt:   time.Now().UTC(),
	}, nil
}
