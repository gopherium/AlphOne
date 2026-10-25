// SPDX-License-Identifier: Elastic-2.0

package postgres_test

import (
	"errors"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"

	"github.com/gopherium/alphone/internal/contact"
	"github.com/gopherium/alphone/internal/postgres"
)

// matchBound is the most identifiers a matcher under test answers in one call.
const matchBound = 3

// ownedAddress is a contact's name beside the one address it owns.
type ownedAddress struct {
	name       string
	channel    contact.Channel
	identifier string
}

// storedOwnerOf stores a contact owning the address.
func storedOwnerOf(t *testing.T, store *postgres.ContactStore, owned ownedAddress) contact.Contact {
	t.Helper()
	owner := mustContact(t, owned.name)
	held := mustIdentity(t, owner.ID, owned.channel, owned.identifier)
	if err := store.CreateContactWithIdentity(t.Context(), owner, held); err != nil {
		t.Fatalf("CreateContactWithIdentity() error = %v, want nil", err)
	}
	return owner
}

func TestMatchingFindsTheOwnersOfAddressesWrittenInAnotherCase(t *testing.T) {
	t.Parallel()

	store := postgres.NewContactStore(newTestPool(t))
	maria := storedOwnerOf(t, store, ownedAddress{"Maria Perez", "email", "maria.perez@example.com"})
	ada := storedOwnerOf(t, store, ownedAddress{"Ada Lovelace", "email", "ada@example.com"})

	got, err := contact.NewMatcher(store, matchBound).MatchIdentities(
		t.Context(), "email", []string{"Maria.Perez@Example.COM", " ADA@example.com "},
	)

	if err != nil {
		t.Fatalf("MatchIdentities() error = %v, want nil", err)
	}
	want := map[string]contact.Contact{"maria.perez@example.com": maria, "ada@example.com": ada}
	if diff := cmp.Diff(want, got, cmpopts.EquateApproxTime(time.Microsecond)); diff != "" {
		t.Errorf("MatchIdentities() mismatch (-want +got):\n%s", diff)
	}
}

func TestMatchingLeavesUnknownAddressesOut(t *testing.T) {
	t.Parallel()

	store := postgres.NewContactStore(newTestPool(t))
	maria := storedOwnerOf(t, store, ownedAddress{"Maria Perez", "email", "maria@example.com"})
	storedOwnerOf(t, store, ownedAddress{"Grace Hopper", "phone", "+184467235"})

	got, err := contact.NewMatcher(store, matchBound).MatchIdentities(
		t.Context(), "email", []string{"maria@example.com", "nobody@example.com", "+184467235"},
	)

	if err != nil {
		t.Fatalf("MatchIdentities() error = %v, want nil", err)
	}
	if len(got) != 1 || got["maria@example.com"].ID != maria.ID {
		t.Errorf("MatchIdentities() = %+v, want only the owner of maria@example.com", got)
	}
}

func TestMatchingAnswersABatchAtTheBound(t *testing.T) {
	t.Parallel()

	store := postgres.NewContactStore(newTestPool(t))
	storedOwnerOf(t, store, ownedAddress{"Maria Perez", "email", "maria@example.com"})

	got, err := contact.NewMatcher(store, matchBound).MatchIdentities(
		t.Context(), "email", []string{"maria@example.com", "ada@example.com", "grace@example.com"},
	)

	if err != nil {
		t.Fatalf("MatchIdentities() at the bound error = %v, want nil", err)
	}
	if len(got) != 1 {
		t.Errorf("MatchIdentities() answered %d owners, want 1", len(got))
	}
}

func TestMatchingRefusesABatchAboveTheBound(t *testing.T) {
	t.Parallel()

	store := postgres.NewContactStore(newTestPool(t))
	storedOwnerOf(t, store, ownedAddress{"Maria Perez", "email", "maria@example.com"})

	got, err := contact.NewMatcher(store, matchBound).MatchIdentities(t.Context(), "email", []string{
		"maria@example.com", "ada@example.com", "grace@example.com", "alan@example.com",
	})

	if !errors.Is(err, contact.ErrTooManyIdentifiers) {
		t.Fatalf("MatchIdentities() error = %v, want %v", err, contact.ErrTooManyIdentifiers)
	}
	if got != nil {
		t.Errorf("MatchIdentities() = %+v beside the refusal, want nothing", got)
	}
}

func TestMatchingReportsAConnectionFailure(t *testing.T) {
	t.Parallel()

	pool := newTestPool(t)
	store := postgres.NewContactStore(pool)
	pool.Close()

	if _, err := store.MatchIdentities(t.Context(), "email", []string{"maria@example.com"}); err == nil {
		t.Error("MatchIdentities() on a closed pool error = nil, want the failure")
	}
}
