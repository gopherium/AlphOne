// SPDX-License-Identifier: Elastic-2.0

package postgres_test

import (
	"errors"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	"github.com/google/uuid"

	"github.com/gopherium/alphone/internal/apitoken"
	"github.com/gopherium/alphone/internal/postgres"
)

func mustMint(t *testing.T, userID uuid.UUID, name string) apitoken.Minted {
	t.Helper()
	minted, err := apitoken.Mint(userID, name)
	if err != nil {
		t.Fatalf("apitoken.Mint() error = %v, want nil", err)
	}
	return minted
}

func TestTokenStoreRoundTrip(t *testing.T) {
	t.Parallel()

	store := postgres.NewTokenStore(newTestPool(t))
	minted := mustMint(t, uuid.Must(uuid.NewV7()), "n8n production")

	if err := store.Create(t.Context(), minted.Token); err != nil {
		t.Fatalf("Create() error = %v, want nil", err)
	}

	got, err := store.ByHash(t.Context(), minted.Token.Hash)
	if err != nil {
		t.Fatalf("ByHash() error = %v, want nil", err)
	}
	if diff := cmp.Diff(minted.Token, got, cmpopts.EquateApproxTime(time.Microsecond)); diff != "" {
		t.Errorf("ByHash() mismatch (-want +got):\n%s", diff)
	}
}

func TestTokenStoreReportsAnUnknownSecret(t *testing.T) {
	t.Parallel()

	store := postgres.NewTokenStore(newTestPool(t))

	_, err := store.ByHash(t.Context(), apitoken.HashSecret("a1_never_minted"))

	if !errors.Is(err, apitoken.ErrNotFound) {
		t.Errorf("ByHash() error = %v, want %v", err, apitoken.ErrNotFound)
	}
}

func TestTokenStoreTouchesLastUsed(t *testing.T) {
	t.Parallel()

	store := postgres.NewTokenStore(newTestPool(t))
	minted := mustMint(t, uuid.Must(uuid.NewV7()), "n8n production")
	if err := store.Create(t.Context(), minted.Token); err != nil {
		t.Fatalf("creating token: %v", err)
	}
	usedAt := time.Now().UTC().Truncate(time.Millisecond)

	if err := store.TouchLastUsed(t.Context(), minted.Token.ID, usedAt); err != nil {
		t.Fatalf("TouchLastUsed() error = %v, want nil", err)
	}

	got, err := store.ByHash(t.Context(), minted.Token.Hash)
	if err != nil {
		t.Fatalf("ByHash() error = %v, want nil", err)
	}
	if !got.LastUsedAt.Equal(usedAt) {
		t.Errorf("LastUsedAt = %v, want %v", got.LastUsedAt, usedAt)
	}
}

func TestTokenStoreListsNewestFirstForOneUserOnly(t *testing.T) {
	t.Parallel()

	store := postgres.NewTokenStore(newTestPool(t))
	owner := uuid.Must(uuid.NewV7())
	stranger := uuid.Must(uuid.NewV7())
	older := mustMint(t, owner, "older")
	newer := mustMint(t, owner, "newer")
	theirs := mustMint(t, stranger, "theirs")
	for _, minted := range []apitoken.Minted{older, newer, theirs} {
		if err := store.Create(t.Context(), minted.Token); err != nil {
			t.Fatalf("creating token %q: %v", minted.Token.Name, err)
		}
	}

	got, err := store.ListForUser(t.Context(), owner)

	if err != nil {
		t.Fatalf("ListForUser() error = %v, want nil", err)
	}
	names := make([]string, 0, len(got))
	for _, token := range got {
		names = append(names, token.Name)
	}
	if diff := cmp.Diff([]string{"newer", "older"}, names); diff != "" {
		t.Errorf("ListForUser() names mismatch (-want +got):\n%s", diff)
	}
}

func TestTokenStoreRevokesOneTokenOfItsOwner(t *testing.T) {
	t.Parallel()

	store := postgres.NewTokenStore(newTestPool(t))
	owner := uuid.Must(uuid.NewV7())
	minted := mustMint(t, owner, "n8n production")
	if err := store.Create(t.Context(), minted.Token); err != nil {
		t.Fatalf("creating token: %v", err)
	}

	if err := store.Revoke(t.Context(), owner, minted.Token.ID); err != nil {
		t.Fatalf("Revoke() error = %v, want nil", err)
	}

	if _, err := store.ByHash(t.Context(), minted.Token.Hash); !errors.Is(err, apitoken.ErrNotFound) {
		t.Errorf("ByHash() after revoke error = %v, want %v", err, apitoken.ErrNotFound)
	}
}

func TestTokenStoreReportsConnectionFailure(t *testing.T) {
	t.Parallel()

	pool := newTestPool(t)
	store := postgres.NewTokenStore(pool)
	owner := uuid.Must(uuid.NewV7())
	minted := mustMint(t, owner, "n8n production")
	pool.Close()

	if err := store.Create(t.Context(), minted.Token); err == nil {
		t.Error("Create() on closed pool error = nil, want error")
	}
	if _, err := store.ByHash(t.Context(), minted.Token.Hash); err == nil || errors.Is(err, apitoken.ErrNotFound) {
		t.Errorf("ByHash() on closed pool error = %v, want a non-ErrNotFound error", err)
	}
	if err := store.TouchLastUsed(t.Context(), minted.Token.ID, time.Now()); err == nil {
		t.Error("TouchLastUsed() on closed pool error = nil, want error")
	}
	if _, err := store.ListForUser(t.Context(), owner); err == nil {
		t.Error("ListForUser() on closed pool error = nil, want error")
	}
	if err := store.Revoke(t.Context(), owner, minted.Token.ID); err == nil || errors.Is(err, apitoken.ErrNotFound) {
		t.Errorf("Revoke() on closed pool error = %v, want a non-ErrNotFound error", err)
	}
}

func TestTokenStoreRefusesToRevokeSomeoneElsesToken(t *testing.T) {
	t.Parallel()

	store := postgres.NewTokenStore(newTestPool(t))
	minted := mustMint(t, uuid.Must(uuid.NewV7()), "n8n production")
	if err := store.Create(t.Context(), minted.Token); err != nil {
		t.Fatalf("creating token: %v", err)
	}

	err := store.Revoke(t.Context(), uuid.Must(uuid.NewV7()), minted.Token.ID)

	if !errors.Is(err, apitoken.ErrNotFound) {
		t.Errorf("Revoke() error = %v, want %v", err, apitoken.ErrNotFound)
	}
	if _, err := store.ByHash(t.Context(), minted.Token.Hash); err != nil {
		t.Errorf("ByHash() after refused revoke error = %v, want the token intact", err)
	}
}
