// SPDX-License-Identifier: Elastic-2.0

package graphres_test

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/gopherium/gouncer"
	"github.com/gopherium/gouncer/authkit"
	authkitpg "github.com/gopherium/gouncer/authkit/postgres"
	"github.com/gopherium/gouncer/authkit/testkit"

	"github.com/gopherium/alphone/internal/graphres"
	"github.com/gopherium/alphone/internal/postgres"
	"github.com/gopherium/alphone/internal/role"
	"github.com/gopherium/alphone/internal/tenant"
	"github.com/gopherium/alphone/sdk"
)

// newPlacingResolver returns a resolver inviting over the real stores on pool.
func newPlacingResolver(pool *pgxpool.Pool) *graphres.Resolver {
	users := authkitpg.NewUserStore(pool)
	config := authkit.InvitesConfig{Store: users}
	return &graphres.Resolver{
		Version:    "9.9.9",
		Invites:    authkit.NewInvites(config),
		Onboarding: postgres.NewOnboarding(pool, config),
		Accounts:   users,
		Tenants:    postgres.NewTenantStore(pool),
		PublicURL:  "https://crm.example.com",
	}
}

// inviteFrom runs the invite mutation for maria@example.com as an admin standing in tenantID.
func inviteFrom(t *testing.T, resolver *graphres.Resolver, tenantID uuid.UUID) {
	t.Helper()
	actor := authkit.Identity{ID: uuid.Must(uuid.NewV7()), Role: role.Admin.String()}
	client := newDecoratedGraphClient(t, resolver, func(ctx context.Context) context.Context {
		return sdk.WithTenant(authkit.WithIdentity(ctx, actor), tenantID)
	})
	var response struct {
		Invite invitePayload `json:"invite"`
	}
	client.MustPost(
		`mutation { invite(email: "maria@example.com", name: "Maria Perez") { delivered activationLink } }`,
		&response,
	)
}

// invitedAccount reads the account the invite created.
func invitedAccount(t *testing.T, pool *pgxpool.Pool) gouncer.User {
	t.Helper()
	held, err := authkitpg.NewUserStore(pool).UserByEmail(t.Context(), "maria@example.com")
	if err != nil {
		t.Fatalf("UserByEmail() error = %v, want the invited account", err)
	}
	return held
}

func TestInvitePlacesTheNewAccountInTheActorsTenant(t *testing.T) {
	t.Parallel()

	pool := newTestPool(t)
	acme := uuid.Must(uuid.NewV7())
	if _, err := pool.Exec(t.Context(),
		"INSERT INTO core.tenants (id, name) VALUES ($1, $2)", acme, "Acme"); err != nil {
		t.Fatalf("seeding the tenant: %v", err)
	}
	resolver := newPlacingResolver(pool)

	inviteFrom(t, resolver, acme)

	placed, err := postgres.NewTenantStore(pool).TenantForUser(t.Context(), invitedAccount(t, pool).ID)
	if err != nil {
		t.Fatalf("TenantForUser() error = %v, want nil", err)
	}
	if placed.ID != acme {
		t.Errorf("the invited account stands in %s, want the actor's tenant %s", placed.ID, acme)
	}
}

func TestInviteLeavesTheInviteeUnplacedWhenTheActorStandsInTheDefault(t *testing.T) {
	t.Parallel()

	pool := newTestPool(t)
	resolver := newPlacingResolver(pool)

	inviteFrom(t, resolver, tenant.DefaultID)

	placed, err := postgres.NewTenantStore(pool).TenantsOf(t.Context(), []uuid.UUID{invitedAccount(t, pool).ID})
	if err != nil {
		t.Fatalf("TenantsOf() error = %v, want nil", err)
	}
	if len(placed) != 0 {
		t.Errorf("TenantsOf() = %v, want no membership row when the actor stands in the default tenant", placed)
	}
}

func TestInviteHandsTheActorsTenantToTheOnboarding(t *testing.T) {
	t.Parallel()

	resolver := newInviteResolver(testkit.NewStore(), nil)
	acme := uuid.Must(uuid.NewV7())

	inviteFrom(t, resolver, acme)

	placing, ok := resolver.Onboarding.(*placingInvites)
	if !ok {
		t.Fatalf("Onboarding is %T, want the recording fake", resolver.Onboarding)
	}
	if handed := placing.placedIn("maria@example.com"); handed != acme {
		t.Errorf("the onboarding was handed tenant %s, want the actor's %s", handed, acme)
	}
}
