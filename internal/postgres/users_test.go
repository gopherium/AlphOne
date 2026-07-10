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
}
