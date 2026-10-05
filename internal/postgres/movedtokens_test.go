// SPDX-License-Identifier: Elastic-2.0

package postgres_test

import (
	"database/sql"
	"testing"

	"github.com/google/uuid"
	"github.com/peterldowns/pgtestdb"
	"github.com/pressly/goose/v3"

	"github.com/gopherium/alphone/internal/tenant"
	"github.com/gopherium/alphone/internal/testdb"
)

// movedTokensVersion is the migration moving each token of the default tenant to its owner's tenant.
const movedTokensVersion = 20

// beforeTheTokenMove returns a fresh database rolled back to the schema before the token move, and its provider.
func beforeTheTokenMove(t *testing.T) (*sql.DB, *goose.Provider) {
	t.Helper()
	if testing.Short() {
		t.Skip("skipping database test in short mode")
	}
	db := openDatabase(t, pgtestdb.Custom(t, testdb.Config(), testdb.Migrator()).URL())
	provider := coreProvider(t, db)
	if _, err := provider.DownTo(t.Context(), movedTokensVersion-1); err != nil {
		t.Fatalf("rolling back to the schema before the token move: %v", err)
	}
	return db, provider
}

// storeTenant stores one tenant called name and returns its identifier.
func storeTenant(t *testing.T, db *sql.DB, name string) uuid.UUID {
	t.Helper()
	id := uuid.Must(uuid.NewV7())
	if _, err := db.ExecContext(t.Context(), "INSERT INTO core.tenants (id, name) VALUES ($1, $2)", id, name); err != nil {
		t.Fatalf("storing the tenant %s: %v", name, err)
	}
	return id
}

// placeUser stands the user in the tenant.
func placeUser(t *testing.T, db *sql.DB, userID, tenantID uuid.UUID) {
	t.Helper()
	if _, err := db.ExecContext(t.Context(),
		"INSERT INTO core.tenant_members (user_id, tenant_id) VALUES ($1, $2)", userID, tenantID); err != nil {
		t.Fatalf("placing the user: %v", err)
	}
}

// storeTokenIn stores one token of the user in the tenant and returns its identifier.
func storeTokenIn(t *testing.T, db *sql.DB, userID, tenantID uuid.UUID) uuid.UUID {
	t.Helper()
	id := uuid.Must(uuid.NewV7())
	if _, err := db.ExecContext(t.Context(),
		`INSERT INTO core.api_tokens (id, user_id, name, token_hash, scopes, created_at, tenant_id)
		VALUES ($1, $2, 'automation', $3, '*', now(), $4)`, id, userID, id.String(), tenantID); err != nil {
		t.Fatalf("storing the token: %v", err)
	}
	return id
}

// tenantOfToken returns the tenant the token is stored in.
func tenantOfToken(t *testing.T, db *sql.DB, tokenID uuid.UUID) uuid.UUID {
	t.Helper()
	var held uuid.UUID
	if err := db.QueryRowContext(t.Context(),
		"SELECT tenant_id FROM core.api_tokens WHERE id = $1", tokenID).Scan(&held); err != nil {
		t.Fatalf("reading the tenant of the token: %v", err)
	}
	return held
}

// rowVersion returns the transaction that last wrote the row of the token.
func rowVersion(t *testing.T, db *sql.DB, tokenID uuid.UUID) string {
	t.Helper()
	var written string
	if err := db.QueryRowContext(t.Context(),
		"SELECT xmin::text FROM core.api_tokens WHERE id = $1", tokenID).Scan(&written); err != nil {
		t.Fatalf("reading the row version of the token: %v", err)
	}
	return written
}

func TestMigrationMovesEachDefaultTenantTokenOfAPlacedOwnerToItsTenant(t *testing.T) {
	t.Parallel()

	db, provider := beforeTheTokenMove(t)
	acme := storeTenant(t, db, "Acme")
	beta := storeTenant(t, db, "Beta")
	placed := seedUser(t, db, "maria.perez@example.com")
	unplaced := seedUser(t, db, "admin@example.com")
	placeUser(t, db, placed, acme)
	tokens := map[string]struct{ token, want uuid.UUID }{
		"a placed owner's token in the default tenant": {storeTokenIn(t, db, placed, tenant.DefaultID), acme},
		"a placed owner's token in its own tenant":     {storeTokenIn(t, db, placed, acme), acme},
		"a placed owner's token in another tenant":     {storeTokenIn(t, db, placed, beta), beta},
		"an unplaced owner's token":                    {storeTokenIn(t, db, unplaced, tenant.DefaultID), tenant.DefaultID},
	}

	if _, err := provider.UpTo(t.Context(), movedTokensVersion); err != nil {
		t.Fatalf("applying the token move: %v", err)
	}

	for name, tc := range tokens {
		if got := tenantOfToken(t, db, tc.token); got != tc.want {
			t.Errorf("%s stands in the tenant %s, want %s", name, got, tc.want)
		}
	}
}

func TestMigrationLeavesTheTokenOfAnOwnerPlacedInTheDefaultTenantUnwritten(t *testing.T) {
	t.Parallel()

	db, provider := beforeTheTokenMove(t)
	owner := seedUser(t, db, "maria.perez@example.com")
	placeUser(t, db, owner, tenant.DefaultID)
	token := storeTokenIn(t, db, owner, tenant.DefaultID)
	written := rowVersion(t, db, token)

	if _, err := provider.UpTo(t.Context(), movedTokensVersion); err != nil {
		t.Fatalf("applying the token move: %v", err)
	}

	if got := tenantOfToken(t, db, token); got != tenant.DefaultID {
		t.Errorf("the token stands in the tenant %s, want it kept in the default tenant %s", got, tenant.DefaultID)
	}
	if again := rowVersion(t, db, token); again != written {
		t.Errorf("the token's row was written again by transaction %s after %s, want it untouched", again, written)
	}
}

func TestRollingTheTokenMoveBackAndApplyingItAgainKeepsTheTokenInItsOwnersTenant(t *testing.T) {
	t.Parallel()

	db, provider := beforeTheTokenMove(t)
	acme := storeTenant(t, db, "Acme")
	placed := seedUser(t, db, "maria.perez@example.com")
	placeUser(t, db, placed, acme)
	token := storeTokenIn(t, db, placed, tenant.DefaultID)
	if _, err := provider.UpTo(t.Context(), movedTokensVersion); err != nil {
		t.Fatalf("applying the token move: %v", err)
	}

	if _, err := provider.DownTo(t.Context(), movedTokensVersion-1); err != nil {
		t.Fatalf("rolling the token move back: %v", err)
	}
	rolledBack := tenantOfToken(t, db, token)
	if _, err := provider.UpTo(t.Context(), movedTokensVersion); err != nil {
		t.Fatalf("applying the token move again: %v", err)
	}

	if rolledBack != acme {
		t.Errorf("the token stands in the tenant %s after the rollback, want it kept in %s", rolledBack, acme)
	}
	if again := tenantOfToken(t, db, token); again != acme {
		t.Errorf("the token stands in the tenant %s after a second run, want it kept in %s", again, acme)
	}
}
