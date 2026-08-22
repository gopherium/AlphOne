// SPDX-License-Identifier: Elastic-2.0

package postgres_test

import (
	"database/sql"
	"errors"
	"io/fs"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	"github.com/google/uuid"
	"github.com/peterldowns/pgtestdb"
	"github.com/pressly/goose/v3"

	"github.com/gopherium/alphone/internal/apitoken"
	"github.com/gopherium/alphone/internal/postgres"
	"github.com/gopherium/alphone/internal/testdb"
)

func mustMint(t *testing.T, userID uuid.UUID, name string) apitoken.Minted {
	t.Helper()
	return mustMintScoped(t, userID, name, apitoken.Full(), apitoken.Never)
}

// mustMintScoped returns a token minted with the given scopes and lifetime.
func mustMintScoped(
	t *testing.T, userID uuid.UUID, name string, scopes apitoken.Scopes, ttl time.Duration,
) apitoken.Minted {
	t.Helper()
	minted, err := apitoken.Mint(userID, name, scopes, ttl)
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

func TestTokenStoreRoundTripsTheScopesAndTheExpiry(t *testing.T) {
	t.Parallel()

	store := postgres.NewTokenStore(newTestPool(t))
	scopes := apitoken.ParseScopes("contacts:read tasks:write")
	minted := mustMintScoped(t, uuid.Must(uuid.NewV7()), "automation", scopes, 30*24*time.Hour)
	if err := store.Create(t.Context(), minted.Token); err != nil {
		t.Fatalf("Create() error = %v, want nil", err)
	}

	got, err := store.ByHash(t.Context(), minted.Token.Hash)

	if err != nil {
		t.Fatalf("ByHash() error = %v, want nil", err)
	}
	if want := "contacts:read tasks:write"; got.Scopes.String() != want {
		t.Errorf("Scopes = %q, want %q", got.Scopes.String(), want)
	}
	if !got.ExpiresAt.Equal(minted.Token.ExpiresAt.Truncate(time.Microsecond)) {
		t.Errorf("ExpiresAt = %v, want %v", got.ExpiresAt, minted.Token.ExpiresAt)
	}
}

func TestTokenStoreLeavesTheExpiryOpenForATokenWithoutOne(t *testing.T) {
	t.Parallel()

	store := postgres.NewTokenStore(newTestPool(t))
	minted := mustMint(t, uuid.Must(uuid.NewV7()), "forever")
	if err := store.Create(t.Context(), minted.Token); err != nil {
		t.Fatalf("Create() error = %v, want nil", err)
	}

	got, err := store.ByHash(t.Context(), minted.Token.Hash)

	if err != nil {
		t.Fatalf("ByHash() error = %v, want nil", err)
	}
	if !got.ExpiresAt.IsZero() {
		t.Errorf("ExpiresAt = %v, want zero, the token never expires", got.ExpiresAt)
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

// scopedTokensVersion is the migration granting api_tokens their scopes and expiry.
const scopedTokensVersion = 12

// movedRolesVersion is the migration moving every tier onto the account.
const movedRolesVersion = 14

// coreProvider returns a goose provider over the core migrations of db.
func coreProvider(t *testing.T, db *sql.DB) *goose.Provider {
	t.Helper()
	source, err := fs.Sub(postgres.Migrations, "migrations")
	if err != nil {
		t.Fatalf("reading the migration source: %v", err)
	}
	provider, err := goose.NewProvider(goose.DialectPostgres, db, source)
	if err != nil {
		t.Fatalf("building the migration provider: %v", err)
	}
	return provider
}

func TestMigrationGrantsFullScopeToATokenMintedBeforeScopesExisted(t *testing.T) {
	t.Parallel()
	if testing.Short() {
		t.Skip("skipping database test in short mode")
	}

	cfg := pgtestdb.Custom(t, testdb.Config(), testdb.Migrator())
	db, err := sql.Open("pgx", cfg.URL())
	if err != nil {
		t.Fatalf("opening the database: %v", err)
	}
	defer func() { _ = db.Close() }()
	provider := coreProvider(t, db)
	if _, err := provider.DownTo(t.Context(), scopedTokensVersion-1); err != nil {
		t.Fatalf("rolling back to the schema before scopes: %v", err)
	}
	legacy := uuid.Must(uuid.NewV7())
	if _, err := db.ExecContext(t.Context(),
		`INSERT INTO core.api_tokens (id, user_id, name, token_hash, created_at)
		VALUES ($1, $2, 'minted before scopes', 'legacy-hash', now())`,
		legacy, uuid.Must(uuid.NewV7())); err != nil {
		t.Fatalf("storing the legacy token: %v", err)
	}

	if _, err := provider.UpTo(t.Context(), scopedTokensVersion); err != nil {
		t.Fatalf("applying the scopes migration: %v", err)
	}

	var scopes string
	var expiresAt sql.NullTime
	if err := db.QueryRowContext(t.Context(),
		"SELECT scopes, expires_at FROM core.api_tokens WHERE id = $1", legacy,
	).Scan(&scopes, &expiresAt); err != nil {
		t.Fatalf("reading the migrated token: %v", err)
	}
	if scopes != apitoken.Wildcard {
		t.Errorf("scopes = %q, want %q, an existing token keeps the authority it had", scopes, apitoken.Wildcard)
	}
	if expiresAt.Valid {
		t.Errorf("expires_at = %v, want NULL, an existing token keeps living", expiresAt.Time)
	}
}

func TestRollingTheScopesMigrationBackRevokesEveryToken(t *testing.T) {
	t.Parallel()
	if testing.Short() {
		t.Skip("skipping database test in short mode")
	}

	cfg := pgtestdb.Custom(t, testdb.Config(), testdb.Migrator())
	db, err := sql.Open("pgx", cfg.URL())
	if err != nil {
		t.Fatalf("opening the database: %v", err)
	}
	defer func() { _ = db.Close() }()
	narrow := uuid.Must(uuid.NewV7())
	if _, err := db.ExecContext(t.Context(),
		`INSERT INTO core.api_tokens (id, user_id, name, token_hash, scopes, created_at, expires_at)
		VALUES ($1, $2, 'narrow and spent', 'narrow-hash', 'contacts:read', now(), now() - interval '1 day')`,
		narrow, uuid.Must(uuid.NewV7())); err != nil {
		t.Fatalf("storing the narrow token: %v", err)
	}
	provider := coreProvider(t, db)

	if _, err := provider.DownTo(t.Context(), scopedTokensVersion-1); err != nil {
		t.Fatalf("rolling back the scopes migration: %v", err)
	}
	if _, err := provider.UpTo(t.Context(), scopedTokensVersion); err != nil {
		t.Fatalf("re-applying the scopes migration: %v", err)
	}

	var surviving int
	if err := db.QueryRowContext(t.Context(),
		"SELECT count(*) FROM core.api_tokens WHERE id = $1", narrow).Scan(&surviving); err != nil {
		t.Fatalf("counting the surviving tokens: %v", err)
	}
	if surviving != 0 {
		t.Error("the narrow token outlived the rollback, want it revoked, the grandfather default would widen it")
	}
}

func TestMigrationRefusesToGrantFullScopeToATokenMintedAfterwards(t *testing.T) {
	t.Parallel()

	pool := newTestPool(t)

	_, err := pool.Exec(t.Context(),
		`INSERT INTO core.api_tokens (id, user_id, name, token_hash, created_at)
		VALUES ($1, $2, 'forgot its scopes', 'forgetful-hash', now())`,
		uuid.Must(uuid.NewV7()), uuid.Must(uuid.NewV7()))

	if err == nil {
		t.Error("insert without scopes error = nil, want a refusal, the grandfather default is spent")
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
