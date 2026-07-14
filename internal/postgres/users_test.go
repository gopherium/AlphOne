// SPDX-License-Identifier: Elastic-2.0

package postgres_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"

	"github.com/gopherium/gouncer"

	"github.com/gopherium/alphone/internal/postgres"
)

func mustUser(t *testing.T) gouncer.User {
	t.Helper()
	u, err := gouncer.NewUser("ada@example.com", "Ada Lovelace", "correct horse battery")
	if err != nil {
		t.Fatalf("gouncer.NewUser() error = %v, want nil", err)
	}
	return u
}

func mustSession(t *testing.T, u gouncer.User) gouncer.Session {
	t.Helper()
	s, err := gouncer.NewSession(u.ID)
	if err != nil {
		t.Fatalf("gouncer.NewSession() error = %v, want nil", err)
	}
	return s
}

func TestUserStoreRoundTrip(t *testing.T) {
	t.Parallel()

	store := postgres.NewUserStore(newTestPool(t))
	ada := mustUser(t)

	if err := store.CreateUser(t.Context(), ada); err != nil {
		t.Fatalf("CreateUser() error = %v, want nil", err)
	}
	got, err := store.UserByEmail(t.Context(), "ada@example.com")

	if err != nil {
		t.Fatalf("UserByEmail() error = %v, want nil", err)
	}
	if diff := cmp.Diff(ada, got, cmpopts.EquateApproxTime(time.Microsecond)); diff != "" {
		t.Errorf("UserByEmail() mismatch (-want +got):\n%s", diff)
	}
}

func TestUserStoreMissingUser(t *testing.T) {
	t.Parallel()

	store := postgres.NewUserStore(newTestPool(t))

	_, err := store.UserByEmail(t.Context(), "nobody@example.com")

	if !errors.Is(err, gouncer.ErrUserNotFound) {
		t.Errorf("UserByEmail() error = %v, want %v", err, gouncer.ErrUserNotFound)
	}
}

func TestUserStoreRejectsDuplicateEmail(t *testing.T) {
	t.Parallel()

	store := postgres.NewUserStore(newTestPool(t))
	if err := store.CreateUser(t.Context(), mustUser(t)); err != nil {
		t.Fatalf("CreateUser() error = %v, want nil", err)
	}

	err := store.CreateUser(t.Context(), mustUser(t))

	if !errors.Is(err, gouncer.ErrEmailTaken) {
		t.Errorf("CreateUser() error = %v, want %v", err, gouncer.ErrEmailTaken)
	}
}

func TestUserStoreSessionRoundTrip(t *testing.T) {
	t.Parallel()

	store := postgres.NewUserStore(newTestPool(t))
	ada := mustUser(t)
	if err := store.CreateUser(t.Context(), ada); err != nil {
		t.Fatalf("CreateUser() error = %v, want nil", err)
	}
	session := mustSession(t, ada)

	if err := store.CreateSession(t.Context(), session); err != nil {
		t.Fatalf("CreateSession() error = %v, want nil", err)
	}
	got, err := store.UserBySession(t.Context(), gouncer.HashToken(session.Token), time.Now().UTC())

	if err != nil {
		t.Fatalf("UserBySession() error = %v, want nil", err)
	}
	if diff := cmp.Diff(ada, got, cmpopts.EquateApproxTime(time.Microsecond)); diff != "" {
		t.Errorf("UserBySession() mismatch (-want +got):\n%s", diff)
	}
}

func TestUserStoreSessionLookupRejectsUnusableSessions(t *testing.T) {
	t.Parallel()

	t.Run("unknown token", func(t *testing.T) {
		t.Parallel()

		store := postgres.NewUserStore(newTestPool(t))

		_, err := store.UserBySession(t.Context(), gouncer.HashToken("unknown"), time.Now().UTC())

		if !errors.Is(err, gouncer.ErrSessionNotFound) {
			t.Errorf("UserBySession() error = %v, want %v", err, gouncer.ErrSessionNotFound)
		}
	})

	t.Run("expired session", func(t *testing.T) {
		t.Parallel()

		store := postgres.NewUserStore(newTestPool(t))
		ada := mustUser(t)
		if err := store.CreateUser(t.Context(), ada); err != nil {
			t.Fatalf("CreateUser() error = %v, want nil", err)
		}
		session := mustSession(t, ada)
		session.ExpiresAt = time.Now().UTC().Add(-time.Hour)
		if err := store.CreateSession(t.Context(), session); err != nil {
			t.Fatalf("CreateSession() error = %v, want nil", err)
		}

		_, err := store.UserBySession(t.Context(), gouncer.HashToken(session.Token), time.Now().UTC())

		if !errors.Is(err, gouncer.ErrSessionNotFound) {
			t.Errorf("UserBySession() error = %v, want %v", err, gouncer.ErrSessionNotFound)
		}
	})

	t.Run("disabled user", func(t *testing.T) {
		t.Parallel()

		store := postgres.NewUserStore(newTestPool(t))
		ada := mustUser(t)
		ada.Disabled = true
		if err := store.CreateUser(t.Context(), ada); err != nil {
			t.Fatalf("CreateUser() error = %v, want nil", err)
		}
		session := mustSession(t, ada)
		if err := store.CreateSession(t.Context(), session); err != nil {
			t.Fatalf("CreateSession() error = %v, want nil", err)
		}

		_, err := store.UserBySession(t.Context(), gouncer.HashToken(session.Token), time.Now().UTC())

		if !errors.Is(err, gouncer.ErrSessionNotFound) {
			t.Errorf("UserBySession() error = %v, want %v", err, gouncer.ErrSessionNotFound)
		}
	})
}

func TestUserStoreDeleteSession(t *testing.T) {
	t.Parallel()

	store := postgres.NewUserStore(newTestPool(t))
	ada := mustUser(t)
	if err := store.CreateUser(t.Context(), ada); err != nil {
		t.Fatalf("CreateUser() error = %v, want nil", err)
	}
	session := mustSession(t, ada)
	if err := store.CreateSession(t.Context(), session); err != nil {
		t.Fatalf("CreateSession() error = %v, want nil", err)
	}

	if err := store.DeleteSession(t.Context(), gouncer.HashToken(session.Token)); err != nil {
		t.Fatalf("DeleteSession() error = %v, want nil", err)
	}

	_, err := store.UserBySession(t.Context(), gouncer.HashToken(session.Token), time.Now().UTC())
	if !errors.Is(err, gouncer.ErrSessionNotFound) {
		t.Errorf("UserBySession() after delete error = %v, want %v", err, gouncer.ErrSessionNotFound)
	}
	if err := store.DeleteSession(t.Context(), gouncer.HashToken(session.Token)); err != nil {
		t.Errorf("DeleteSession() of a deleted session error = %v, want nil", err)
	}
}

func TestUserStoreReportsBackendFailures(t *testing.T) {
	t.Parallel()

	store := postgres.NewUserStore(newTestPool(t))
	ada := mustUser(t)
	canceled, cancel := context.WithCancel(t.Context())
	cancel()

	if err := store.CreateUser(canceled, ada); err == nil {
		t.Error("CreateUser() error = nil, want a backend failure")
	}
	if _, err := store.UserByEmail(canceled, "ada@example.com"); err == nil {
		t.Error("UserByEmail() error = nil, want a backend failure")
	}
	if err := store.CreateSession(canceled, mustSession(t, ada)); err == nil {
		t.Error("CreateSession() error = nil, want a backend failure")
	}
	if _, err := store.UserBySession(canceled, gouncer.HashToken("t"), time.Now().UTC()); err == nil {
		t.Error("UserBySession() error = nil, want a backend failure")
	}
	if err := store.DeleteSession(canceled, gouncer.HashToken("t")); err == nil {
		t.Error("DeleteSession() error = nil, want a backend failure")
	}
	if _, err := store.ListUsers(canceled); err == nil {
		t.Error("ListUsers() error = nil, want a backend failure")
	}
	if err := store.SetUserDisabled(canceled, ada.ID, true); err == nil {
		t.Error("SetUserDisabled() error = nil, want a backend failure")
	}
}

func namedUser(t *testing.T, email, name string) gouncer.User {
	t.Helper()
	u, err := gouncer.NewUser(email, name, "correct horse battery")
	if err != nil {
		t.Fatalf("gouncer.NewUser() error = %v, want nil", err)
	}
	return u
}

func TestUserStoreListUsersOrdersByName(t *testing.T) {
	t.Parallel()

	store := postgres.NewUserStore(newTestPool(t))
	grace := namedUser(t, "grace@example.com", "Grace Hopper")
	ada := namedUser(t, "ada@example.com", "Ada Lovelace")
	for _, u := range []gouncer.User{grace, ada} {
		if err := store.CreateUser(t.Context(), u); err != nil {
			t.Fatalf("CreateUser() error = %v, want nil", err)
		}
	}

	got, err := store.ListUsers(t.Context())

	if err != nil {
		t.Fatalf("ListUsers() error = %v, want nil", err)
	}
	want := []gouncer.User{ada, grace}
	if diff := cmp.Diff(want, got, cmpopts.EquateApproxTime(time.Microsecond)); diff != "" {
		t.Errorf("ListUsers() mismatch (-want +got):\n%s", diff)
	}
}

func TestUserStoreListUsersEmpty(t *testing.T) {
	t.Parallel()

	store := postgres.NewUserStore(newTestPool(t))

	got, err := store.ListUsers(t.Context())

	if err != nil {
		t.Fatalf("ListUsers() error = %v, want nil", err)
	}
	if len(got) != 0 {
		t.Errorf("ListUsers() returned %d users, want 0", len(got))
	}
}

func TestUserStoreSetUserDisabled(t *testing.T) {
	t.Parallel()

	store := postgres.NewUserStore(newTestPool(t))
	ada := mustUser(t)
	if err := store.CreateUser(t.Context(), ada); err != nil {
		t.Fatalf("CreateUser() error = %v, want nil", err)
	}

	if err := store.SetUserDisabled(t.Context(), ada.ID, true); err != nil {
		t.Fatalf("SetUserDisabled(true) error = %v, want nil", err)
	}
	disabled, err := store.UserByEmail(t.Context(), ada.Email)
	if err != nil {
		t.Fatalf("UserByEmail() error = %v, want nil", err)
	}
	if !disabled.Disabled {
		t.Error("user still enabled after SetUserDisabled(true)")
	}

	if err := store.SetUserDisabled(t.Context(), ada.ID, false); err != nil {
		t.Fatalf("SetUserDisabled(false) error = %v, want nil", err)
	}
	enabled, err := store.UserByEmail(t.Context(), ada.Email)
	if err != nil {
		t.Fatalf("UserByEmail() error = %v, want nil", err)
	}
	if enabled.Disabled {
		t.Error("user still disabled after SetUserDisabled(false)")
	}
}

func TestUserStoreSetUserDisabledUnknownUser(t *testing.T) {
	t.Parallel()

	store := postgres.NewUserStore(newTestPool(t))

	err := store.SetUserDisabled(t.Context(), mustUser(t).ID, true)

	if !errors.Is(err, gouncer.ErrUserNotFound) {
		t.Errorf("SetUserDisabled() error = %v, want %v", err, gouncer.ErrUserNotFound)
	}
}
