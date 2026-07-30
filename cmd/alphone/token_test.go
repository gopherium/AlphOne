// SPDX-License-Identifier: Elastic-2.0

package main

import (
	"errors"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/gopherium/gouncer"

	"github.com/gopherium/alphone/internal/apitoken"
	"github.com/gopherium/alphone/internal/postgres"
)

// seedTokenUser provisions the account the token subcommand acts for.
func seedTokenUser(t *testing.T, getenv func(string) string) {
	t.Helper()
	err := createAdmin(
		t.Context(),
		getenv,
		[]string{"-email", "admin@example.com", "-name", "Admin"},
		strings.NewReader("correct horse battery\n"),
		io.Discard,
	)
	if err != nil {
		t.Fatalf("createAdmin() error = %v, want nil", err)
	}
}

func TestTokenCreatePrintsTheSecretOnce(t *testing.T) {
	t.Parallel()

	databaseURL := testDatabaseURL(t)
	getenv := testGetenv(map[string]string{"ALPHONE_DATABASE_URL": databaseURL})
	seedTokenUser(t, getenv)
	var stdout strings.Builder

	err := token(t.Context(), getenv, []string{"create", "-email", "admin@example.com", "-name", "n8n"}, &stdout)

	if err != nil {
		t.Fatalf("token() error = %v, want nil", err)
	}
	printed := stdout.String()
	if !strings.Contains(printed, apitoken.Prefix) {
		t.Fatalf("output = %q, want it to carry the secret", printed)
	}
	secret := ""
	for _, field := range strings.Fields(printed) {
		if strings.HasPrefix(field, apitoken.Prefix) {
			secret = field
		}
	}
	pool, err := pgxpool.New(t.Context(), databaseURL)
	if err != nil {
		t.Fatalf("connecting pool: %v", err)
	}
	t.Cleanup(pool.Close)
	stored, err := postgres.NewTokenStore(pool).ByHash(t.Context(), apitoken.HashSecret(secret))
	if err != nil {
		t.Fatalf("ByHash() error = %v, want the stored token", err)
	}
	if stored.Name != "n8n" {
		t.Errorf("stored name = %q, want %q", stored.Name, "n8n")
	}
}

func TestTokenCreateRejectsAnUnknownEmail(t *testing.T) {
	t.Parallel()

	getenv := testGetenv(map[string]string{"ALPHONE_DATABASE_URL": testDatabaseURL(t)})

	err := token(t.Context(), getenv, []string{"create", "-email", "nobody@example.com", "-name", "n8n"}, io.Discard)

	if !errors.Is(err, gouncer.ErrUserNotFound) {
		t.Errorf("token() error = %v, want %v", err, gouncer.ErrUserNotFound)
	}
}

func TestTokenCreateRejectsABlankName(t *testing.T) {
	t.Parallel()

	getenv := testGetenv(map[string]string{"ALPHONE_DATABASE_URL": testDatabaseURL(t)})
	seedTokenUser(t, getenv)

	err := token(t.Context(), getenv, []string{"create", "-email", "admin@example.com", "-name", "  "}, io.Discard)

	if !errors.Is(err, apitoken.ErrEmptyName) {
		t.Errorf("token() error = %v, want %v", err, apitoken.ErrEmptyName)
	}
}

func TestTokenListNamesEveryTokenWithoutItsSecret(t *testing.T) {
	t.Parallel()

	getenv := testGetenv(map[string]string{"ALPHONE_DATABASE_URL": testDatabaseURL(t)})
	seedTokenUser(t, getenv)
	var created strings.Builder
	args := []string{"create", "-email", "admin@example.com", "-name", "n8n"}
	if err := token(t.Context(), getenv, args, &created); err != nil {
		t.Fatalf("token(create) error = %v, want nil", err)
	}
	var stdout strings.Builder

	err := token(t.Context(), getenv, []string{"list", "-email", "admin@example.com"}, &stdout)

	if err != nil {
		t.Fatalf("token(list) error = %v, want nil", err)
	}
	if !strings.Contains(stdout.String(), "n8n") {
		t.Errorf("output = %q, want it to name the token", stdout.String())
	}
	if strings.Contains(stdout.String(), apitoken.Prefix) {
		t.Errorf("output = %q, want no secret in a listing", stdout.String())
	}
}

func TestTokenRevokeRemovesTheToken(t *testing.T) {
	t.Parallel()

	databaseURL := testDatabaseURL(t)
	getenv := testGetenv(map[string]string{"ALPHONE_DATABASE_URL": databaseURL})
	seedTokenUser(t, getenv)
	var created strings.Builder
	args := []string{"create", "-email", "admin@example.com", "-name", "n8n"}
	if err := token(t.Context(), getenv, args, &created); err != nil {
		t.Fatalf("token(create) error = %v, want nil", err)
	}
	secret := ""
	for _, field := range strings.Fields(created.String()) {
		if strings.HasPrefix(field, apitoken.Prefix) {
			secret = field
		}
	}
	pool, err := pgxpool.New(t.Context(), databaseURL)
	if err != nil {
		t.Fatalf("connecting pool: %v", err)
	}
	t.Cleanup(pool.Close)
	store := postgres.NewTokenStore(pool)
	stored, err := store.ByHash(t.Context(), apitoken.HashSecret(secret))
	if err != nil {
		t.Fatalf("ByHash() error = %v, want the stored token", err)
	}

	revokeArgs := []string{"revoke", "-email", "admin@example.com", "-id", stored.ID.String()}
	if err := token(t.Context(), getenv, revokeArgs, io.Discard); err != nil {
		t.Fatalf("token(revoke) error = %v, want nil", err)
	}

	if _, err := store.ByHash(t.Context(), apitoken.HashSecret(secret)); !errors.Is(err, apitoken.ErrNotFound) {
		t.Errorf("ByHash() after revoke error = %v, want %v", err, apitoken.ErrNotFound)
	}
}

func TestTokenRejectsAnUnknownVerb(t *testing.T) {
	t.Parallel()

	getenv := testGetenv(map[string]string{"ALPHONE_DATABASE_URL": testDatabaseURL(t)})
	seedTokenUser(t, getenv)

	err := token(t.Context(), getenv, []string{"sniff", "-email", "admin@example.com"}, io.Discard)

	if err == nil || !strings.Contains(err.Error(), "unknown command") {
		t.Errorf("token() error = %v, want an unknown verb reported", err)
	}
}

func TestTokenRequiresADatabaseURL(t *testing.T) {
	t.Parallel()

	err := token(t.Context(), testGetenv(map[string]string{}), []string{"list", "-email", "a@example.com"}, io.Discard)

	if err == nil {
		t.Error("token() error = nil, want the missing database url reported")
	}
}

func TestTokenRejectsAnUnparsableRevokeID(t *testing.T) {
	t.Parallel()

	getenv := testGetenv(map[string]string{"ALPHONE_DATABASE_URL": testDatabaseURL(t)})
	seedTokenUser(t, getenv)

	args := []string{"revoke", "-email", "admin@example.com", "-id", "not-a-uuid"}
	err := token(t.Context(), getenv, args, io.Discard)

	if err == nil {
		t.Error("token() error = nil, want the malformed id reported")
	}
}

func TestTokenWithoutAVerb(t *testing.T) {
	t.Parallel()

	err := token(t.Context(), testGetenv(map[string]string{}), nil, io.Discard)

	if err == nil {
		t.Error("token() error = nil, want the missing verb reported")
	}
}

func TestTokenRejectsUnparsableFlags(t *testing.T) {
	t.Parallel()

	err := token(t.Context(), testGetenv(map[string]string{}), []string{"list", "-nope"}, io.Discard)

	if err == nil {
		t.Error("token() error = nil, want the unknown flag reported")
	}
}

func TestTokenRejectsAnUnparsableDatabaseURL(t *testing.T) {
	t.Parallel()

	getenv := testGetenv(map[string]string{"ALPHONE_DATABASE_URL": "://not a url"})

	err := token(t.Context(), getenv, []string{"list", "-email", "admin@example.com"}, io.Discard)

	if err == nil {
		t.Error("token() error = nil, want the malformed url reported")
	}
}

func TestTokenListShowsTheLastUseDate(t *testing.T) {
	t.Parallel()

	databaseURL := testDatabaseURL(t)
	getenv := testGetenv(map[string]string{"ALPHONE_DATABASE_URL": databaseURL})
	seedTokenUser(t, getenv)
	var created strings.Builder
	args := []string{"create", "-email", "admin@example.com", "-name", "n8n"}
	if err := token(t.Context(), getenv, args, &created); err != nil {
		t.Fatalf("token(create) error = %v, want nil", err)
	}
	secret := ""
	for _, field := range strings.Fields(created.String()) {
		if strings.HasPrefix(field, apitoken.Prefix) {
			secret = field
		}
	}
	pool, err := pgxpool.New(t.Context(), databaseURL)
	if err != nil {
		t.Fatalf("connecting pool: %v", err)
	}
	t.Cleanup(pool.Close)
	store := postgres.NewTokenStore(pool)
	stored, err := store.ByHash(t.Context(), apitoken.HashSecret(secret))
	if err != nil {
		t.Fatalf("ByHash() error = %v, want the stored token", err)
	}
	if err := store.TouchLastUsed(t.Context(), stored.ID, time.Now().UTC()); err != nil {
		t.Fatalf("TouchLastUsed() error = %v, want nil", err)
	}
	var stdout strings.Builder

	if err := token(t.Context(), getenv, []string{"list", "-email", "admin@example.com"}, &stdout); err != nil {
		t.Fatalf("token(list) error = %v, want nil", err)
	}

	if strings.Contains(stdout.String(), "last used never") {
		t.Errorf("output = %q, want a real last-use date", stdout.String())
	}
}

// closedTokenStore returns a token store whose pool is already closed.
func closedTokenStore(t *testing.T) *postgres.TokenStore {
	t.Helper()
	pool, err := pgxpool.New(t.Context(), testDatabaseURL(t))
	if err != nil {
		t.Fatalf("connecting pool: %v", err)
	}
	pool.Close()
	return postgres.NewTokenStore(pool)
}

func TestTokenReportsAnUnreachableDatabase(t *testing.T) {
	t.Parallel()

	getenv := testGetenv(map[string]string{"ALPHONE_DATABASE_URL": unreachableDatabaseURL})

	err := token(t.Context(), getenv, []string{"list", "-email", "admin@example.com"}, io.Discard)

	if err == nil {
		t.Error("token() error = nil, want the unreachable database reported")
	}
}

func TestTokenVerbsReportStoreFailures(t *testing.T) {
	t.Parallel()

	store := closedTokenStore(t)
	owner := uuid.Must(uuid.NewV7())

	if err := createToken(t.Context(), store, owner, "n8n", io.Discard); err == nil {
		t.Error("createToken() on a closed pool error = nil, want error")
	}
	if err := listTokens(t.Context(), store, owner, io.Discard); err == nil {
		t.Error("listTokens() on a closed pool error = nil, want error")
	}
	if err := revokeToken(t.Context(), store, owner, uuid.Nil.String(), io.Discard); err == nil {
		t.Error("revokeToken() on a closed pool error = nil, want error")
	}
}
