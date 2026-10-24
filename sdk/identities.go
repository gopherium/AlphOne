// SPDX-License-Identifier: Elastic-2.0

package sdk

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
)

// ErrTooManyIdentifiers reports a match asking for more identifiers than the host answers in one call.
var ErrTooManyIdentifiers = errors.New("sdk: too many identifiers")

// ErrCursorAhead reports a feed cursor past every transaction the database server began, as after a restore elsewhere.
var ErrCursorAhead = errors.New("sdk: cursor ahead of the server")

// ContactMatcher finds the calling tenant's contacts owning many identities in one call.
type ContactMatcher interface {
	// MatchIdentities returns the contact owning each identifier one owns, keyed by the identifier's canonical form.
	MatchIdentities(ctx context.Context, channel Channel, identifiers []string) (map[string]Contact, error)
}

// IdentityCursor marks a place in the order identities were added, its zero value the place before every identity.
type IdentityCursor struct {
	// Transaction is the transaction that added the identity at the place.
	Transaction uint64
	// Identity is the id of the identity at the place.
	Identity uuid.UUID
}

// AddedIdentity is one identity a contact of the calling tenant gained.
type AddedIdentity struct {
	ContactID  uuid.UUID
	Identifier string
	AddedAt    time.Time
}

// IdentityFeed lists the identities the calling tenant's contacts gain, in an order no late commit can skip.
type IdentityFeed interface {
	// IdentitiesAddedSince returns at most limit identities past the cursor, none at the end, and the next cursor.
	IdentitiesAddedSince(
		ctx context.Context, channel Channel, cursor IdentityCursor, limit int,
	) ([]AddedIdentity, IdentityCursor, error)
	// Horizon returns the cursor a new reader starts from.
	Horizon(ctx context.Context) (IdentityCursor, error)
}
