// SPDX-License-Identifier: Elastic-2.0

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

// IdentityExistsError reports a claimed identity and the contact owning it.
type IdentityExistsError struct {
	OwnerID uuid.UUID
}

// Error returns the [ErrIdentityExists] message.
func (e IdentityExistsError) Error() string {
	return ErrIdentityExists.Error()
}

// Is reports whether target is [ErrIdentityExists].
func (e IdentityExistsError) Is(target error) bool {
	return target == ErrIdentityExists
}

// Channel names the communication medium of an identity, such as
// "whatsapp" or "email". Valid values are defined by channel plugins,
// never enumerated by the core.
type Channel string

// normalizeChannel trims surrounding whitespace and lowercases channel.
func normalizeChannel(channel Channel) Channel {
	return Channel(strings.ToLower(strings.TrimSpace(string(channel))))
}

// normalizeIdentifier applies the channel's canonical form to identifier.
func normalizeIdentifier(channel Channel, identifier string) string {
	switch channel {
	case "email":
		return strings.ToLower(identifier)
	case "phone":
		return normalizePhone(identifier)
	default:
		return identifier
	}
}

// normalizePhone reduces a phone number to its digits and a leading plus.
func normalizePhone(identifier string) string {
	digits := strings.Map(keepDigit, identifier)
	if digits == "" || !strings.HasPrefix(identifier, "+") {
		return digits
	}
	return "+" + digits
}

// keepDigit returns r unchanged when it is a decimal digit, dropping the rest.
func keepDigit(r rune) rune {
	if r < '0' || r > '9' {
		return -1
	}
	return r
}

// Identity is a per-channel address of a [Contact]. Identifier is opaque
// to the core and stored exactly as its channel delivered it.
type Identity struct {
	ID          uuid.UUID
	ContactID   uuid.UUID
	Channel     Channel
	Identifier  string
	DisplayName string
	CreatedAt   time.Time
}

// NewIdentity returns an [Identity] owned by contactID. The channel is
// trimmed and lowercased, the identifier is trimmed and then normalized
// to the channel's canonical form, emails lowercase and phones digits
// with a leading plus, other channels store it verbatim.
func NewIdentity(contactID uuid.UUID, channel Channel, identifier, displayName string) (Identity, error) {
	if contactID == uuid.Nil {
		return Identity{}, ErrNilContactID
	}
	normalizedChannel := normalizeChannel(channel)
	if normalizedChannel == "" {
		return Identity{}, ErrEmptyChannel
	}
	trimmedIdentifier := normalizeIdentifier(normalizedChannel, strings.TrimSpace(identifier))
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
