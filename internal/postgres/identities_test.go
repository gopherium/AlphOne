// SPDX-License-Identifier: AGPL-3.0-or-later

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

func TestContactStoreLookupIdentityRoundTrip(t *testing.T) {
	t.Parallel()

	store := postgres.NewContactStore(newTestPool(t))
	maria := mustContact(t, "María Pérez")
	identity := mustIdentity(t, maria.ID, "whatsapp", "184467235@lid")

	if err := store.CreateContactWithIdentity(t.Context(), maria, identity); err != nil {
		t.Fatalf("CreateContactWithIdentity() error = %v, want nil", err)
	}
	got, err := store.LookupIdentity(t.Context(), "whatsapp", "184467235@lid")

	if err != nil {
		t.Fatalf("LookupIdentity() error = %v, want nil", err)
	}
	if diff := cmp.Diff(identity, got, cmpopts.EquateApproxTime(time.Microsecond)); diff != "" {
		t.Errorf("LookupIdentity() mismatch (-want +got):\n%s", diff)
	}
}

func TestContactStoreLookupIdentityMissing(t *testing.T) {
	t.Parallel()

	store := postgres.NewContactStore(newTestPool(t))

	_, err := store.LookupIdentity(t.Context(), "whatsapp", "184467235@lid")

	if !errors.Is(err, contact.ErrIdentityNotFound) {
		t.Fatalf("LookupIdentity() error = %v, want %v", err, contact.ErrIdentityNotFound)
	}
}

func TestContactStoreCreateContactWithIdentityKeepsFirstOwner(t *testing.T) {
	t.Parallel()

	store := postgres.NewContactStore(newTestPool(t))
	maria := mustContact(t, "María Pérez")
	if err := store.CreateContactWithIdentity(t.Context(), maria, mustIdentity(t, maria.ID, "whatsapp", "184467235@lid")); err != nil {
		t.Fatalf("CreateContactWithIdentity() error = %v, want nil", err)
	}

	john := mustContact(t, "John Doe")
	err := store.CreateContactWithIdentity(t.Context(), john, mustIdentity(t, john.ID, "whatsapp", "184467235@lid"))

	if !errors.Is(err, contact.ErrIdentityExists) {
		t.Fatalf("CreateContactWithIdentity() error = %v, want %v", err, contact.ErrIdentityExists)
	}
	if _, err := store.Get(t.Context(), john.ID); !errors.Is(err, contact.ErrNotFound) {
		t.Errorf("Get(john) error = %v, want %v after rollback", err, contact.ErrNotFound)
	}
	got, err := store.LookupIdentity(t.Context(), "whatsapp", "184467235@lid")
	if err != nil {
		t.Fatalf("LookupIdentity() error = %v, want nil", err)
	}
	if got.ContactID != maria.ID {
		t.Errorf("LookupIdentity().ContactID = %s, want first owner %s", got.ContactID, maria.ID)
	}
}

func TestContactStoreCreateContactWithIdentityRejectsReusedIDs(t *testing.T) {
	t.Parallel()

	store := postgres.NewContactStore(newTestPool(t))
	maria := mustContact(t, "María Pérez")
	first := mustIdentity(t, maria.ID, "whatsapp", "184467235@lid")
	if err := store.CreateContactWithIdentity(t.Context(), maria, first); err != nil {
		t.Fatalf("CreateContactWithIdentity() error = %v, want nil", err)
	}

	t.Run("contact id", func(t *testing.T) {
		err := store.CreateContactWithIdentity(t.Context(), maria, mustIdentity(t, maria.ID, "email", "maria@acme.com"))

		if err == nil || errors.Is(err, contact.ErrIdentityExists) {
			t.Fatalf("CreateContactWithIdentity() error = %v, want a non-ErrIdentityExists error", err)
		}
	})

	t.Run("identity id", func(t *testing.T) {
		john := mustContact(t, "John Doe")
		reused := mustIdentity(t, john.ID, "email", "john@acme.com")
		reused.ID = first.ID

		err := store.CreateContactWithIdentity(t.Context(), john, reused)

		if err == nil || errors.Is(err, contact.ErrIdentityExists) {
			t.Fatalf("CreateContactWithIdentity() error = %v, want a non-ErrIdentityExists error", err)
		}
		if _, err := store.Get(t.Context(), john.ID); !errors.Is(err, contact.ErrNotFound) {
			t.Errorf("Get(john) error = %v, want %v after rollback", err, contact.ErrNotFound)
		}
	})
}

func TestContactStoreIdentityConnectionFailure(t *testing.T) {
	t.Parallel()

	pool := newTestPool(t)
	store := postgres.NewContactStore(pool)
	maria := mustContact(t, "María Pérez")
	identity := mustIdentity(t, maria.ID, "whatsapp", "184467235@lid")
	pool.Close()

	if err := store.CreateContactWithIdentity(t.Context(), maria, identity); err == nil || errors.Is(err, contact.ErrIdentityExists) {
		t.Errorf("CreateContactWithIdentity() on closed pool error = %v, want a plain error", err)
	}
	if _, err := store.LookupIdentity(t.Context(), "whatsapp", "184467235@lid"); err == nil || errors.Is(err, contact.ErrIdentityNotFound) {
		t.Errorf("LookupIdentity() on closed pool error = %v, want a plain error", err)
	}
}
