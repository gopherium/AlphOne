// SPDX-License-Identifier: Elastic-2.0

package postgres_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/gopherium/gouncer"
	"github.com/gopherium/gouncer/authkit"
	authkitpg "github.com/gopherium/gouncer/authkit/postgres"

	"github.com/gopherium/alphone/internal/postgres"
	"github.com/gopherium/alphone/internal/tenant"
)

// standingTenant stores a tenant named Acme and returns its id.
func standingTenant(t *testing.T, pool *pgxpool.Pool) uuid.UUID {
	t.Helper()
	acme := uuid.Must(uuid.NewV7())
	if _, err := pool.Exec(t.Context(),
		"INSERT INTO core.tenants (id, name) VALUES ($1, $2)", acme, "Acme"); err != nil {
		t.Fatalf("seeding the tenant: %v", err)
	}
	return acme
}

// refuseAtCommit makes every membership row raise when the transaction commits.
func refuseAtCommit(t *testing.T, pool *pgxpool.Pool) {
	t.Helper()
	statements := []string{
		`CREATE FUNCTION core.refuse_at_commit() RETURNS trigger LANGUAGE plpgsql AS $$
		BEGIN
			RAISE EXCEPTION 'the membership stands refused';
		END;
		$$`,
		`CREATE CONSTRAINT TRIGGER refuse_membership AFTER INSERT ON core.tenant_members
		DEFERRABLE INITIALLY DEFERRED FOR EACH ROW EXECUTE FUNCTION core.refuse_at_commit()`,
	}
	for _, statement := range statements {
		if _, err := pool.Exec(t.Context(), statement); err != nil {
			t.Fatalf("arming the deferred refusal: %v", err)
		}
	}
}

func TestInviteIntoPlacesTheAccountInTheTenant(t *testing.T) {
	t.Parallel()

	pool := newTestPool(t)
	acme := standingTenant(t, pool)
	onboarding := postgres.NewOnboarding(pool, authkit.InvitesConfig{})

	token, err := onboarding.InviteInto(t.Context(), acme, "maria@example.com", "Maria Perez", "member")

	if err != nil {
		t.Fatalf("InviteInto() error = %v, want nil", err)
	}
	placed, err := postgres.NewTenantStore(pool).TenantForUser(t.Context(), token.UserID)
	if err != nil {
		t.Fatalf("TenantForUser() error = %v, want nil", err)
	}
	if placed.ID != acme {
		t.Errorf("the invited account stands in %s, want the Acme tenant %s", placed.ID, acme)
	}
}

func TestInviteIntoLeavesTheDefaultTenantsAccountUnplaced(t *testing.T) {
	t.Parallel()

	pool := newTestPool(t)
	onboarding := postgres.NewOnboarding(pool, authkit.InvitesConfig{})

	token, err := onboarding.InviteInto(
		t.Context(), tenant.DefaultID, "maria@example.com", "Maria Perez", "member")

	if err != nil {
		t.Fatalf("InviteInto() error = %v, want nil", err)
	}
	placed, err := postgres.NewTenantStore(pool).TenantsOf(t.Context(), []uuid.UUID{token.UserID})
	if err != nil {
		t.Fatalf("TenantsOf() error = %v, want nil", err)
	}
	if len(placed) != 0 {
		t.Errorf("TenantsOf() = %v, want no membership row for an invite into the default tenant", placed)
	}
}

func TestInviteIntoLeavesNoAccountWhenThePlacementFails(t *testing.T) {
	t.Parallel()

	pool := newTestPool(t)
	onboarding := postgres.NewOnboarding(pool, authkit.InvitesConfig{})
	unknownTenant := uuid.Must(uuid.NewV7())

	_, err := onboarding.InviteInto(t.Context(), unknownTenant, "maria@example.com", "Maria Perez", "member")

	if err == nil {
		t.Fatal("InviteInto() error = nil, want the placement refused")
	}
	_, err = authkitpg.NewUserStore(pool).UserByEmail(t.Context(), "maria@example.com")
	if !errors.Is(err, gouncer.ErrUserNotFound) {
		t.Errorf("UserByEmail() error = %v, want ErrUserNotFound after the rollback", err)
	}
}

func TestInviteIntoRefusesWhenTheTransactionCannotBegin(t *testing.T) {
	t.Parallel()

	pool := newTestPool(t)
	onboarding := postgres.NewOnboarding(pool, authkit.InvitesConfig{})
	pool.Close()

	_, err := onboarding.InviteInto(
		t.Context(), tenant.DefaultID, "maria@example.com", "Maria Perez", "member")

	if err == nil {
		t.Fatal("InviteInto() error = nil, want the closed pool refused")
	}
}

func TestInviteIntoRefusesWhenTheTransactionCannotCommit(t *testing.T) {
	t.Parallel()

	pool := newTestPool(t)
	acme := standingTenant(t, pool)
	refuseAtCommit(t, pool)
	onboarding := postgres.NewOnboarding(pool, authkit.InvitesConfig{})

	_, err := onboarding.InviteInto(t.Context(), acme, "maria@example.com", "Maria Perez", "member")

	if err == nil || !strings.Contains(err.Error(), "the membership stands refused") {
		t.Fatalf("InviteInto() error = %v, want the refusal the commit raised", err)
	}
	_, err = authkitpg.NewUserStore(pool).UserByEmail(t.Context(), "maria@example.com")
	if !errors.Is(err, gouncer.ErrUserNotFound) {
		t.Errorf("UserByEmail() error = %v, want ErrUserNotFound after the refused commit", err)
	}
}

func TestInviteIntoPassesATakenEmailThrough(t *testing.T) {
	t.Parallel()

	pool := newTestPool(t)
	acme := standingTenant(t, pool)
	held, err := gouncer.NewInvitedUser("maria@example.com", "Maria Perez")
	if err != nil {
		t.Fatalf("gouncer.NewInvitedUser() error = %v, want nil", err)
	}
	if err := authkitpg.NewUserStore(pool).CreateUser(t.Context(), held); err != nil {
		t.Fatalf("CreateUser() error = %v, want nil", err)
	}
	onboarding := postgres.NewOnboarding(pool, authkit.InvitesConfig{})

	_, err = onboarding.InviteInto(t.Context(), acme, "maria@example.com", "Maria Perez", "member")

	if !errors.Is(err, gouncer.ErrEmailTaken) {
		t.Errorf("InviteInto() error = %v, want ErrEmailTaken passed through", err)
	}
}
