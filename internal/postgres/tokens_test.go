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
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/peterldowns/pgtestdb"
	"github.com/pressly/goose/v3"

	"github.com/gopherium/gouncer"
	authkitpg "github.com/gopherium/gouncer/authkit/postgres"

	"github.com/gopherium/alphone/internal/apitoken"
	"github.com/gopherium/alphone/internal/postgres"
	"github.com/gopherium/alphone/internal/tenant"
	"github.com/gopherium/alphone/internal/testdb"
	"github.com/gopherium/alphone/sdk"
)

// mustMint returns a token minted for the user with full scopes and no expiry.
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

// storedOwner stores an invited account at email for tokens to belong to and answers its id.
func storedOwner(t *testing.T, pool *pgxpool.Pool, email string) uuid.UUID {
	t.Helper()
	held, err := gouncer.NewInvitedUser(email, "Maria Perez")
	if err != nil {
		t.Fatalf("gouncer.NewInvitedUser() error = %v, want nil", err)
	}
	if err := authkitpg.NewUserStore(pool).CreateUser(t.Context(), held); err != nil {
		t.Fatalf("CreateUser() error = %v, want nil", err)
	}
	return held.ID
}

// heldToken is what a test reads back of one token ListEvery answers.
type heldToken struct {
	Owner    string
	TenantID uuid.UUID
	Name     string
}

func TestListEveryTokenCoversEveryTenant(t *testing.T) {
	t.Parallel()

	pool := newTestPool(t)
	store := postgres.NewTokenStore(pool)
	acme := uuid.Must(uuid.NewV7())
	if _, err := pool.Exec(t.Context(), "INSERT INTO core.tenants (id, name) VALUES ($1, $2)", acme, "Acme"); err != nil {
		t.Fatalf("storing the tenant: %v", err)
	}
	inDefault := mustMint(t, storedOwner(t, pool, "admin@example.com"), "automation")
	inAcme := mustMint(t, storedOwner(t, pool, "maria.perez@example.com"), "reporting")
	ownerless := mustMint(t, uuid.Must(uuid.NewV7()), "ownerless")
	for _, stored := range []struct {
		tenantID uuid.UUID
		minted   apitoken.Minted
	}{{tenant.DefaultID, inDefault}, {acme, inAcme}, {tenant.DefaultID, ownerless}} {
		if err := store.Create(sdk.WithTenant(t.Context(), stored.tenantID), stored.minted.Token); err != nil {
			t.Fatalf("creating token %q: %v", stored.minted.Token.Name, err)
		}
	}

	got, err := store.ListEvery(sdk.WithTenant(t.Context(), acme))

	if err != nil {
		t.Fatalf("ListEvery() error = %v, want nil", err)
	}
	held := map[uuid.UUID]heldToken{}
	for _, owned := range got {
		held[owned.Token.ID] = heldToken{Owner: owned.Owner, TenantID: owned.TenantID, Name: owned.Token.Name}
	}
	want := map[uuid.UUID]heldToken{
		inDefault.Token.ID: {Owner: "admin@example.com", TenantID: tenant.DefaultID, Name: "automation"},
		inAcme.Token.ID:    {Owner: "maria.perez@example.com", TenantID: acme, Name: "reporting"},
		ownerless.Token.ID: {Owner: "", TenantID: tenant.DefaultID, Name: "ownerless"},
	}
	if diff := cmp.Diff(want, held); diff != "" || len(got) != len(want) {
		t.Errorf("ListEvery() answered %d tokens, mismatch (-want +got):\n%s", len(got), diff)
	}
}

func TestListEveryTokenReportsOwnersItCannotRead(t *testing.T) {
	t.Parallel()

	pool := newTestPool(t)
	store := postgres.NewTokenStore(pool)
	if err := store.Create(t.Context(), mustMint(t, uuid.Must(uuid.NewV7()), "automation").Token); err != nil {
		t.Fatalf("creating token: %v", err)
	}
	if _, err := pool.Exec(t.Context(), "DROP TABLE auth.users CASCADE"); err != nil {
		t.Fatalf("dropping the accounts: %v", err)
	}

	got, err := store.ListEvery(t.Context())

	if err == nil || got != nil {
		t.Errorf("ListEvery() = %v, %v, want no tokens and the owner read failure", got, err)
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

func TestTokenStoreFindsAndRevokesATokenItsOwnerLeftInAnotherTenant(t *testing.T) {
	t.Parallel()

	pool := newTestPool(t)
	store := postgres.NewTokenStore(pool)
	owner := storedOwner(t, pool, "maria.perez@example.com")
	minted := mustMint(t, owner, "reporting")
	if err := store.Create(standingIn(t, tenant.DefaultID), minted.Token); err != nil {
		t.Fatalf("creating token: %v", err)
	}
	acme := seededTenant(t, pool)
	if _, err := pool.Exec(t.Context(),
		"INSERT INTO core.tenant_members (user_id, tenant_id) VALUES ($1, $2)", owner, acme); err != nil {
		t.Fatalf("placing the owner: %v", err)
	}
	standing := standingIn(t, acme)
	if held, err := store.ListForUser(standing, owner); err != nil || len(held) != 0 {
		t.Fatalf("ListForUser() in the owner's tenant = %v, %v, want no token there", held, err)
	}

	found, err := store.FindInAnyTenant(standing, owner, minted.Token.ID)

	if err != nil {
		t.Fatalf("FindInAnyTenant() error = %v, want the token left in the default tenant", err)
	}
	if diff := cmp.Diff(minted.Token, found, cmpopts.EquateApproxTime(time.Microsecond)); diff != "" {
		t.Errorf("FindInAnyTenant() mismatch (-want +got):\n%s", diff)
	}
	if err := store.RevokeInAnyTenant(standing, owner, minted.Token.ID); err != nil {
		t.Fatalf("RevokeInAnyTenant() error = %v, want nil", err)
	}
	if _, err := store.ByHash(t.Context(), minted.Token.Hash); !errors.Is(err, apitoken.ErrNotFound) {
		t.Errorf("ByHash() after RevokeInAnyTenant() error = %v, want %v", err, apitoken.ErrNotFound)
	}
}

func TestTokenStoreReachesNoTokenInAnyTenantUnlessTheOwnerAndTheIDBothMatch(t *testing.T) {
	t.Parallel()

	pool := newTestPool(t)
	store := postgres.NewTokenStore(pool)
	owner := storedOwner(t, pool, "maria.perez@example.com")
	minted := mustMint(t, owner, "reporting")
	if err := store.Create(standingIn(t, seededTenant(t, pool)), minted.Token); err != nil {
		t.Fatalf("creating token: %v", err)
	}
	lookups := map[string]struct {
		userID, tokenID uuid.UUID
	}{
		"the token id under another account": {storedOwner(t, pool, "admin@example.com"), minted.Token.ID},
		"an id no token carries":             {owner, uuid.Must(uuid.NewV7())},
	}

	for condition, lookup := range lookups {
		_, found := store.FindInAnyTenant(t.Context(), lookup.userID, lookup.tokenID)
		revoked := store.RevokeInAnyTenant(t.Context(), lookup.userID, lookup.tokenID)

		if !errors.Is(found, apitoken.ErrNotFound) || !errors.Is(revoked, apitoken.ErrNotFound) {
			t.Errorf("%s: FindInAnyTenant() error = %v, RevokeInAnyTenant() error = %v, want %v from both",
				condition, found, revoked, apitoken.ErrNotFound)
		}
	}
	if _, err := store.ByHash(t.Context(), minted.Token.Hash); err != nil {
		t.Errorf("ByHash() after the refused revokes error = %v, want the token intact", err)
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
	if _, err := store.ListEvery(t.Context()); err == nil {
		t.Error("ListEvery() on closed pool error = nil, want error")
	}
	if err := store.Revoke(t.Context(), owner, minted.Token.ID); err == nil || errors.Is(err, apitoken.ErrNotFound) {
		t.Errorf("Revoke() on closed pool error = %v, want a non-ErrNotFound error", err)
	}
}

func TestTokenStoreReportsConnectionFailureInAnyTenant(t *testing.T) {
	t.Parallel()

	pool := newTestPool(t)
	store := postgres.NewTokenStore(pool)
	owner := uuid.Must(uuid.NewV7())
	pool.Close()

	_, found := store.FindInAnyTenant(t.Context(), owner, uuid.Must(uuid.NewV7()))
	revoked := store.RevokeInAnyTenant(t.Context(), owner, uuid.Must(uuid.NewV7()))

	for name, err := range map[string]error{"FindInAnyTenant": found, "RevokeInAnyTenant": revoked} {
		if err == nil || errors.Is(err, apitoken.ErrNotFound) {
			t.Errorf("%s() on closed pool error = %v, want a non-ErrNotFound error", name, err)
		}
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
		t.Error("insert without scopes error = nil, want an error, the grandfather default is spent")
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
