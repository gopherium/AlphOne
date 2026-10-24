// SPDX-License-Identifier: Elastic-2.0

package main

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"

	"github.com/gopherium/alphone/internal/contact"
	"github.com/gopherium/alphone/sdk"
)

var errStore = errors.New("the contact store is unreachable")

// failingStore answers every contact read and write with errStore.
type failingStore struct{}

// Get returns an empty contact and errStore.
func (failingStore) Get(context.Context, uuid.UUID) (contact.Contact, error) {
	return contact.Contact{}, errStore
}

// LookupIdentity returns an empty identity and errStore.
func (failingStore) LookupIdentity(context.Context, contact.Channel, string) (contact.Identity, error) {
	return contact.Identity{}, errStore
}

// CreateContactWithIdentity returns errStore.
func (failingStore) CreateContactWithIdentity(context.Context, contact.Contact, contact.Identity) error {
	return errStore
}

// CreateContactWithIdentities returns errStore.
func (failingStore) CreateContactWithIdentities(context.Context, contact.Contact, []contact.Identity) error {
	return errStore
}

// MatchIdentities returns no contact and errStore.
func (failingStore) MatchIdentities(context.Context, contact.Channel, []string) (map[string]contact.Contact, error) {
	return nil, errStore
}

// IdentitiesAddedSince returns no identity, the cursor it was given and errStore.
func (failingStore) IdentitiesAddedSince(
	_ context.Context, _ contact.Channel, after contact.Cursor, _ int,
) ([]contact.AddedIdentity, contact.Cursor, error) {
	return nil, after, errStore
}

// IdentityHorizon returns the zero cursor and errStore.
func (failingStore) IdentityHorizon(context.Context) (contact.Cursor, error) {
	return contact.Cursor{}, errStore
}

// IdentityCursorAhead returns false and errStore.
func (failingStore) IdentityCursorAhead(context.Context, contact.Cursor) (bool, error) {
	return false, errStore
}

// newDirectoryBridge returns a directory bridge whose store always fails.
func newDirectoryBridge() directoryBridge {
	return directoryBridge{resolver: contact.NewResolver(failingStore{})}
}

// newMatcherBridge returns a matcher bridge answering two identifiers a call over a store that always fails.
func newMatcherBridge() matcherBridge {
	return matcherBridge{matcher: contact.NewMatcher(failingStore{}, 2)}
}

// newIdentityFeedBridge returns a feed bridge answering two identities a call over a store that always fails.
func newIdentityFeedBridge() identityFeedBridge {
	return identityFeedBridge{feed: contact.NewFeed(failingStore{}, 2)}
}

// someCursor returns a cursor past the start of the feed.
func someCursor() sdk.IdentityCursor {
	return sdk.IdentityCursor{Transaction: 7, Identity: uuid.Must(uuid.NewV7())}
}

func TestMatcherBridgeMarksTheRefusals(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		channel     sdk.Channel
		identifiers []string
		want        error
	}{
		"a blank channel":           {" ", []string{"maria@example.com"}, sdk.ErrInvalidContact},
		"a phone holding no digits": {"phone", []string{"n/a"}, sdk.ErrInvalidContact},
		"a batch above the bound": {
			"email", []string{"maria@example.com", "ada@example.com", "grace@example.com"}, sdk.ErrTooManyIdentifiers,
		},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			matched, err := newMatcherBridge().MatchIdentities(t.Context(), tt.channel, tt.identifiers)

			if !errors.Is(err, tt.want) {
				t.Errorf("MatchIdentities() error = %v, want a plugin to read it as %v", err, tt.want)
			}
			if matched != nil {
				t.Errorf("MatchIdentities() = %+v on a refusal, want nothing", matched)
			}
		})
	}
}

func TestMatcherBridgeLeavesAStoreFailureUnmarked(t *testing.T) {
	t.Parallel()

	_, err := newMatcherBridge().MatchIdentities(t.Context(), "email", []string{"maria@example.com"})

	if !errors.Is(err, errStore) {
		t.Fatalf("MatchIdentities() error = %v, want %v", err, errStore)
	}
	if errors.Is(err, sdk.ErrInvalidContact) || errors.Is(err, sdk.ErrTooManyIdentifiers) {
		t.Errorf("a store failure was marked as a refusal %v, so a plugin would drop the batch rather than retry", err)
	}
}

func TestIdentityFeedBridgeMarksABlankChannelAndKeepsTheCursor(t *testing.T) {
	t.Parallel()

	cursor := someCursor()

	added, next, err := newIdentityFeedBridge().IdentitiesAddedSince(t.Context(), " ", cursor, 1)

	if !errors.Is(err, sdk.ErrInvalidContact) {
		t.Errorf("IdentitiesAddedSince() error = %v, want a plugin to read it as %v", err, sdk.ErrInvalidContact)
	}
	if added != nil || next != cursor {
		t.Errorf("IdentitiesAddedSince() = %+v at %+v on a refusal, want nothing at %+v", added, next, cursor)
	}
}

// aheadFeedStore answers an empty page for every cursor and holds each one past the server.
type aheadFeedStore struct{}

// IdentitiesAddedSince returns no identity and the cursor it was given.
func (aheadFeedStore) IdentitiesAddedSince(
	_ context.Context, _ contact.Channel, after contact.Cursor, _ int,
) ([]contact.AddedIdentity, contact.Cursor, error) {
	return nil, after, nil
}

// IdentityHorizon returns the zero cursor.
func (aheadFeedStore) IdentityHorizon(context.Context) (contact.Cursor, error) {
	return contact.Cursor{}, nil
}

// IdentityCursorAhead returns true.
func (aheadFeedStore) IdentityCursorAhead(context.Context, contact.Cursor) (bool, error) {
	return true, nil
}

func TestIdentityFeedBridgeMarksACursorPastTheServerAndKeepsIt(t *testing.T) {
	t.Parallel()

	cursor := someCursor()
	bridge := identityFeedBridge{feed: contact.NewFeed(aheadFeedStore{}, 2)}

	added, next, err := bridge.IdentitiesAddedSince(t.Context(), "email", cursor, 1)

	if !errors.Is(err, sdk.ErrCursorAhead) {
		t.Errorf("IdentitiesAddedSince() error = %v, want a plugin to read it as %v", err, sdk.ErrCursorAhead)
	}
	if added != nil || next != cursor {
		t.Errorf("IdentitiesAddedSince() = %+v at %+v on a refusal, want nothing at %+v", added, next, cursor)
	}
}

func TestIdentityFeedBridgeLeavesAStoreFailureUnmarked(t *testing.T) {
	t.Parallel()

	bridge, cursor := newIdentityFeedBridge(), someCursor()

	_, next, err := bridge.IdentitiesAddedSince(t.Context(), "email", cursor, 1)
	if !errors.Is(err, errStore) || errors.Is(err, sdk.ErrInvalidContact) {
		t.Errorf("IdentitiesAddedSince() error = %v, want %v left unmarked", err, errStore)
	}
	if next != cursor {
		t.Errorf("IdentitiesAddedSince() moved the cursor to %+v on a failure, want it kept at %+v", next, cursor)
	}
	if _, err := bridge.Horizon(t.Context()); !errors.Is(err, errStore) {
		t.Errorf("Horizon() error = %v, want %v", err, errStore)
	}
}

func TestDirectoryBridgeMarksUnusableDetails(t *testing.T) {
	t.Parallel()

	tests := map[string]func(*testing.T, directoryBridge) error{
		"a phone holding no digits": func(t *testing.T, b directoryBridge) error {
			_, _, err := b.FindByIdentity(t.Context(), "phone", "n/a")
			return err
		},
		"a name that is only whitespace": func(t *testing.T, b directoryBridge) error {
			_, _, err := b.CreateWithIdentities(t.Context(), "   ",
				[]sdk.Identity{{Channel: "email", Identifier: "maria@example.com"}})
			return err
		},
		"an identifier that normalizes away": func(t *testing.T, b directoryBridge) error {
			_, _, err := b.CreateWithIdentities(t.Context(), "Maria Perez",
				[]sdk.Identity{{Channel: "phone", Identifier: "n/a"}})
			return err
		},
	}

	for name, call := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			err := call(t, newDirectoryBridge())

			if !errors.Is(err, sdk.ErrInvalidContact) {
				t.Errorf("error = %v, want a plugin to read it as %v", err, sdk.ErrInvalidContact)
			}
		})
	}
}

func TestDirectoryBridgeLeavesAStoreFailureUnmarked(t *testing.T) {
	t.Parallel()

	_, _, err := newDirectoryBridge().FindByIdentity(t.Context(), "email", "maria@example.com")

	if !errors.Is(err, errStore) {
		t.Fatalf("error = %v, want %v", err, errStore)
	}
	if errors.Is(err, sdk.ErrInvalidContact) {
		t.Error("a store failure was marked invalid, so a plugin would fail the row rather than stop")
	}
}

func TestResolverBridgeMarksUnusableDetails(t *testing.T) {
	t.Parallel()

	bridge := resolverBridge{resolver: contact.NewResolver(failingStore{})}

	_, err := bridge.Resolve(t.Context(), "phone", "  ", "Maria Perez")

	if !errors.Is(err, sdk.ErrInvalidContact) {
		t.Errorf("error = %v, want a plugin to read it as %v", err, sdk.ErrInvalidContact)
	}
}
