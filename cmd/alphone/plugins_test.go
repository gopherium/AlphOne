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

func (failingStore) Get(context.Context, uuid.UUID) (contact.Contact, error) {
	return contact.Contact{}, errStore
}

func (failingStore) LookupIdentity(context.Context, contact.Channel, string) (contact.Identity, error) {
	return contact.Identity{}, errStore
}

func (failingStore) CreateContactWithIdentity(context.Context, contact.Contact, contact.Identity) error {
	return errStore
}

func (failingStore) CreateContactWithIdentities(context.Context, contact.Contact, []contact.Identity) error {
	return errStore
}

// newDirectoryBridge returns a directory bridge whose store always fails.
func newDirectoryBridge() directoryBridge {
	return directoryBridge{resolver: contact.NewResolver(failingStore{})}
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
