// SPDX-License-Identifier: Elastic-2.0

package contact

import (
	"context"
	"errors"
	"fmt"
)

// ErrTooManyIdentifiers reports a batch holding more identifiers than a match answers at once.
var ErrTooManyIdentifiers = errors.New("contact: too many identifiers")

// DefaultMatchMax is the most identifiers one match answers when the host names no bound.
const DefaultMatchMax = 100

// MatchStore finds the contacts owning many identities in one read.
type MatchStore interface {
	MatchIdentities(ctx context.Context, channel Channel, identifiers []string) (map[string]Contact, error)
}

// Matcher finds the contacts owning a batch of identities, refusing a batch above its bound.
type Matcher struct {
	store MatchStore
	bound int
}

// NewMatcher returns a [Matcher] over store answering at most bound identifiers a call.
func NewMatcher(store MatchStore, bound int) *Matcher {
	return &Matcher{store: store, bound: bound}
}

// MatchIdentities returns the contact owning each identifier on channel that one owns, keyed by its canonical form.
func (m *Matcher) MatchIdentities(
	ctx context.Context, channel Channel, identifiers []string,
) (map[string]Contact, error) {
	if len(identifiers) > m.bound {
		return nil, fmt.Errorf("%w: %d asked, at most %d", ErrTooManyIdentifiers, len(identifiers), m.bound)
	}
	normalizedChannel := normalizeChannel(channel)
	if normalizedChannel == "" {
		return nil, ErrEmptyChannel
	}
	canonical := make([]string, 0, len(identifiers))
	for _, identifier := range identifiers {
		_, key, err := identityKey(normalizedChannel, identifier)
		if err != nil {
			return nil, err
		}
		canonical = append(canonical, key)
	}
	return m.store.MatchIdentities(ctx, normalizedChannel, canonical)
}
