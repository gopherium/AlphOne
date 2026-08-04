// SPDX-License-Identifier: Elastic-2.0

package postgres_test

import (
	"errors"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	"github.com/google/uuid"

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
	if err := store.CreateContactWithIdentity(
		t.Context(),
		maria,
		mustIdentity(t, maria.ID, "whatsapp", "184467235@lid"),
	); err != nil {
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

func TestContactStoreAddIdentityAttachesToAnExistingContact(t *testing.T) {
	t.Parallel()

	store := postgres.NewContactStore(newTestPool(t))
	maria := mustContact(t, "María Pérez")
	if err := store.CreateContactWithIdentity(
		t.Context(), maria, mustIdentity(t, maria.ID, "whatsapp", "184467235@lid"),
	); err != nil {
		t.Fatalf("CreateContactWithIdentity() error = %v, want nil", err)
	}
	email, err := contact.NewIdentity(maria.ID, "email", " Maria.Perez@Example.COM ", "")
	if err != nil {
		t.Fatalf("NewIdentity() error = %v, want nil", err)
	}

	if err := store.AddIdentity(t.Context(), email); err != nil {
		t.Fatalf("AddIdentity() error = %v, want nil", err)
	}

	identities, err := store.ListContactIdentities(t.Context(), maria.ID)
	if err != nil {
		t.Fatalf("ListContactIdentities() error = %v, want nil", err)
	}
	if len(identities) != 2 {
		t.Fatalf("ListContactIdentities() returned %d identities, want 2", len(identities))
	}
	stored, err := store.LookupIdentity(t.Context(), "email", "maria.perez@example.com")
	if err != nil {
		t.Fatalf("LookupIdentity() error = %v, want nil", err)
	}
	if stored.Identifier != "maria.perez@example.com" {
		t.Errorf("stored identifier = %q, want the normalized bytes", stored.Identifier)
	}
	if stored.ContactID != maria.ID {
		t.Errorf("stored ContactID = %s, want %s", stored.ContactID, maria.ID)
	}
}

func TestContactStoreAddIdentityNamesTheOwnerOnConflict(t *testing.T) {
	t.Parallel()

	store := postgres.NewContactStore(newTestPool(t))
	maria := mustContact(t, "María Pérez")
	if err := store.CreateContactWithIdentity(
		t.Context(), maria, mustIdentity(t, maria.ID, "email", "maria@example.com"),
	); err != nil {
		t.Fatalf("CreateContactWithIdentity() error = %v, want nil", err)
	}
	john := mustContact(t, "John Doe")
	if err := store.CreateContactWithIdentity(
		t.Context(), john, mustIdentity(t, john.ID, "whatsapp", "155500011@lid"),
	); err != nil {
		t.Fatalf("CreateContactWithIdentity() error = %v, want nil", err)
	}

	err := store.AddIdentity(t.Context(), mustIdentity(t, john.ID, "email", "maria@example.com"))

	if !errors.Is(err, contact.ErrIdentityExists) {
		t.Fatalf("AddIdentity() error = %v, want %v", err, contact.ErrIdentityExists)
	}
	var exists contact.IdentityExistsError
	if !errors.As(err, &exists) {
		t.Fatalf("AddIdentity() error = %v, want a contact.IdentityExistsError", err)
	}
	if exists.OwnerID != maria.ID {
		t.Errorf("IdentityExistsError.OwnerID = %s, want the owner %s", exists.OwnerID, maria.ID)
	}
	identities, err := store.ListContactIdentities(t.Context(), john.ID)
	if err != nil {
		t.Fatalf("ListContactIdentities() error = %v, want nil", err)
	}
	if len(identities) != 1 {
		t.Errorf("ListContactIdentities(john) returned %d identities, want the original 1", len(identities))
	}
}

func TestContactStoreAddIdentityRequiresTheContact(t *testing.T) {
	t.Parallel()

	store := postgres.NewContactStore(newTestPool(t))
	ghost := mustIdentity(t, uuid.Must(uuid.NewV7()), "email", "maria@example.com")

	err := store.AddIdentity(t.Context(), ghost)

	if !errors.Is(err, contact.ErrNotFound) {
		t.Fatalf("AddIdentity() error = %v, want %v", err, contact.ErrNotFound)
	}
}

func TestContactStoreDeleteIdentity(t *testing.T) {
	t.Parallel()

	store := postgres.NewContactStore(newTestPool(t))
	maria := mustContact(t, "María Pérez")
	if err := store.CreateContactWithIdentity(
		t.Context(), maria, mustIdentity(t, maria.ID, "whatsapp", "184467235@lid"),
	); err != nil {
		t.Fatalf("CreateContactWithIdentity() error = %v, want nil", err)
	}
	email := mustIdentity(t, maria.ID, "email", "maria@example.com")
	if err := store.AddIdentity(t.Context(), email); err != nil {
		t.Fatalf("AddIdentity() error = %v, want nil", err)
	}

	wrongContactID := uuid.Must(uuid.NewV7())
	if err := store.DeleteIdentity(t.Context(), wrongContactID, email.ID); !errors.Is(err, contact.ErrIdentityNotFound) {
		t.Fatalf("DeleteIdentity() under the wrong contact error = %v, want %v", err, contact.ErrIdentityNotFound)
	}

	if err := store.DeleteIdentity(t.Context(), maria.ID, email.ID); err != nil {
		t.Fatalf("DeleteIdentity() error = %v, want nil", err)
	}

	identities, err := store.ListContactIdentities(t.Context(), maria.ID)
	if err != nil {
		t.Fatalf("ListContactIdentities() error = %v, want nil", err)
	}
	if len(identities) != 1 {
		t.Fatalf("ListContactIdentities() returned %d identities, want 1 after delete", len(identities))
	}
	if err := store.DeleteIdentity(t.Context(), maria.ID, email.ID); !errors.Is(err, contact.ErrIdentityNotFound) {
		t.Errorf("DeleteIdentity() second call error = %v, want %v", err, contact.ErrIdentityNotFound)
	}
}

func TestContactStoreCreateContactWithIdentitiesCommitsAllOrNothing(t *testing.T) {
	t.Parallel()

	store := postgres.NewContactStore(newTestPool(t))

	t.Run("both identities land", func(t *testing.T) {
		maria := mustContact(t, "María Pérez")
		identities := []contact.Identity{
			mustIdentity(t, maria.ID, "email", "maria@example.com"),
			mustIdentity(t, maria.ID, "phone", "+184467235"),
		}

		if err := store.CreateContactWithIdentities(t.Context(), maria, identities); err != nil {
			t.Fatalf("CreateContactWithIdentities() error = %v, want nil", err)
		}

		stored, err := store.ListContactIdentities(t.Context(), maria.ID)
		if err != nil {
			t.Fatalf("ListContactIdentities() error = %v, want nil", err)
		}
		if len(stored) != 2 {
			t.Errorf("ListContactIdentities() returned %d identities, want 2", len(stored))
		}
	})

	t.Run("a claimed identity rolls the contact back", func(t *testing.T) {
		john := mustContact(t, "John Doe")
		identities := []contact.Identity{
			mustIdentity(t, john.ID, "phone", "+155500011"),
			mustIdentity(t, john.ID, "email", "maria@example.com"),
		}

		err := store.CreateContactWithIdentities(t.Context(), john, identities)

		var exists contact.IdentityExistsError
		if !errors.As(err, &exists) {
			t.Fatalf("CreateContactWithIdentities() error = %v, want a contact.IdentityExistsError", err)
		}
		if _, err := store.Get(t.Context(), john.ID); !errors.Is(err, contact.ErrNotFound) {
			t.Errorf("Get(john) error = %v, want %v after rollback", err, contact.ErrNotFound)
		}
		if _, err := store.LookupIdentity(t.Context(), "phone", "+155500011"); !errors.Is(err, contact.ErrIdentityNotFound) {
			t.Errorf("LookupIdentity(phone) error = %v, want %v after rollback", err, contact.ErrIdentityNotFound)
		}
	})
}

func TestContactStoreIdentityWriteConnectionFailure(t *testing.T) {
	t.Parallel()

	pool := newTestPool(t)
	store := postgres.NewContactStore(pool)
	maria := mustContact(t, "María Pérez")
	identity := mustIdentity(t, maria.ID, "email", "maria@example.com")
	pool.Close()

	if err := store.AddIdentity(t.Context(), identity); err == nil ||
		errors.Is(err, contact.ErrIdentityExists) || errors.Is(err, contact.ErrNotFound) {
		t.Errorf("AddIdentity() on closed pool error = %v, want a plain error", err)
	}
	if err := store.DeleteIdentity(t.Context(), maria.ID, identity.ID); err == nil ||
		errors.Is(err, contact.ErrIdentityNotFound) {
		t.Errorf("DeleteIdentity() on closed pool error = %v, want a plain error", err)
	}
}

func TestContactStoreIdentityConnectionFailure(t *testing.T) {
	t.Parallel()

	pool := newTestPool(t)
	store := postgres.NewContactStore(pool)
	maria := mustContact(t, "María Pérez")
	identity := mustIdentity(t, maria.ID, "whatsapp", "184467235@lid")
	pool.Close()

	if err := store.CreateContactWithIdentity(
		t.Context(),
		maria,
		identity,
	); err == nil || errors.Is(err, contact.ErrIdentityExists) {
		t.Errorf("CreateContactWithIdentity() on closed pool error = %v, want a plain error", err)
	}
	if _, err := store.LookupIdentity(
		t.Context(),
		"whatsapp",
		"184467235@lid",
	); err == nil || errors.Is(err, contact.ErrIdentityNotFound) {
		t.Errorf("LookupIdentity() on closed pool error = %v, want a plain error", err)
	}
}
