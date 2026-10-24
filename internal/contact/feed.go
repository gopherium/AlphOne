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

// ErrCursorStale reports a cursor this database server cannot continue.
var ErrCursorStale = errors.New("contact: cursor stale")

// Cursor marks a place in the order identities were added, its zero value the place before every identity.
type Cursor struct {
	// Server identifies the PostgreSQL server that numbered the transaction, zero in the zero cursor.
	Server int64
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

// CursorCheck is what the database server answers about a cursor.
type CursorCheck struct {
	// Server identifies the PostgreSQL server that answered.
	Server int64
	// Ahead reports whether the cursor names a transaction past every one the server has begun.
	Ahead bool
}

// FeedStore lists identities in the order they were added.
type FeedStore interface {
	IdentitiesAddedSince(
		ctx context.Context, channel Channel, after Cursor, limit int,
	) ([]AddedIdentity, Cursor, error)
	IdentityHorizon(ctx context.Context) (Cursor, error)
	IdentityCursorCheck(ctx context.Context, cursor Cursor) (CursorCheck, error)
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
	server, err := f.checkCursor(ctx, after)
	if err != nil {
		return nil, after, err
	}
	added, next, err := f.store.IdentitiesAddedSince(ctx, normalizedChannel, after, min(limit, f.bound))
	if err != nil || len(added) == 0 {
		return nil, after, err
	}
	next.Server = server
	return added, next, nil
}

// checkCursor returns the server's identity, or [ErrCursorStale] when the server cannot continue the cursor.
func (f *Feed) checkCursor(ctx context.Context, after Cursor) (int64, error) {
	check, err := f.store.IdentityCursorCheck(ctx, after)
	if err != nil {
		return 0, err
	}
	if after != (Cursor{}) && after.Server != check.Server {
		return 0, fmt.Errorf("%w: the cursor names the server %d, not this server %d",
			ErrCursorStale, after.Server, check.Server)
	}
	if check.Ahead {
		return 0, fmt.Errorf("%w: transaction %d is past the server's count", ErrCursorStale, after.Transaction)
	}
	return check.Server, nil
}

// Horizon returns the cursor a new reader starts from.
func (f *Feed) Horizon(ctx context.Context) (Cursor, error) {
	return f.store.IdentityHorizon(ctx)
}
