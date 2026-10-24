// SPDX-License-Identifier: Elastic-2.0

package contact

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
)

// ErrInvalidLimit reports a page asked to hold fewer than one identity.
var ErrInvalidLimit = errors.New("contact: invalid limit")

// ErrCursorAhead reports a cursor naming a transaction past every one the database server has begun.
var ErrCursorAhead = errors.New("contact: cursor ahead of the server")

// Cursor marks a place in the order identities were added, its zero value the place before every identity.
type Cursor struct {
	// Transaction is the transaction that added the identity at the place.
	Transaction uint64
	// Identity is the id of the identity at the place.
	Identity uuid.UUID
}

// AddedIdentity is one identity the feed answers.
type AddedIdentity struct {
	ContactID  uuid.UUID
	Identifier string
	AddedAt    time.Time
}

// FeedStore lists identities in the order they were added.
type FeedStore interface {
	IdentitiesAddedSince(
		ctx context.Context, channel Channel, after Cursor, limit int,
	) ([]AddedIdentity, Cursor, error)
	IdentityHorizon(ctx context.Context) (Cursor, error)
	IdentityCursorAhead(ctx context.Context, cursor Cursor) (bool, error)
}

// Feed lists the identities each channel gains, in an order no late commit can skip.
type Feed struct {
	store FeedStore
	bound int
}

// NewFeed returns a [Feed] over store answering at most bound identities a call.
func NewFeed(store FeedStore, bound int) *Feed {
	return &Feed{store: store, bound: bound}
}

// IdentitiesAddedSince returns at most limit identities past the cursor, within the bound, and the next cursor.
func (f *Feed) IdentitiesAddedSince(
	ctx context.Context, channel Channel, after Cursor, limit int,
) ([]AddedIdentity, Cursor, error) {
	normalizedChannel := normalizeChannel(channel)
	if normalizedChannel == "" {
		return nil, after, ErrEmptyChannel
	}
	if limit < 1 {
		return nil, after, ErrInvalidLimit
	}
	added, next, err := f.store.IdentitiesAddedSince(ctx, normalizedChannel, after, min(limit, f.bound))
	if err != nil || len(added) > 0 {
		return added, next, err
	}
	return nil, after, f.refuseCursorAhead(ctx, after)
}

// refuseCursorAhead reports [ErrCursorAhead] when the cursor names a transaction the server has not begun.
func (f *Feed) refuseCursorAhead(ctx context.Context, after Cursor) error {
	ahead, err := f.store.IdentityCursorAhead(ctx, after)
	if err != nil {
		return err
	}
	if ahead {
		return fmt.Errorf("%w: transaction %d", ErrCursorAhead, after.Transaction)
	}
	return nil
}

// Horizon returns the cursor a new reader starts from.
func (f *Feed) Horizon(ctx context.Context) (Cursor, error) {
	return f.store.IdentityHorizon(ctx)
}
