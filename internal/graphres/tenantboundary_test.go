// SPDX-License-Identifier: Elastic-2.0

package graphres_test

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/gopherium/gouncer/authkit"
	"github.com/gopherium/gouncer/authkit/testkit"

	"github.com/gopherium/alphone/internal/role"
	"github.com/gopherium/alphone/internal/tenant"
)

// standingTenants places named users in a tenant, answering the default for the rest.
type standingTenants struct {
	standing map[uuid.UUID]uuid.UUID
}

// TenantForUser answers the tenant the user stands in.
func (s standingTenants) TenantForUser(_ context.Context, userID uuid.UUID) (tenant.Tenant, error) {
	held, ok := s.standing[userID]
	if !ok {
		held = tenant.DefaultID
	}
	return tenant.Tenant{ID: held, Name: "Standing"}, nil
}

// TenantsOf answers the tenant of every named user a row places.
func (s standingTenants) TenantsOf(
	_ context.Context, ids []uuid.UUID,
) (map[uuid.UUID]uuid.UUID, error) {
	held := make(map[uuid.UUID]uuid.UUID)
	for _, id := range ids {
		if placed, ok := s.standing[id]; ok {
			held[id] = placed
		}
	}
	return held, nil
}

// roledUser adds an account standing in a role to the store.
func roledUser(t *testing.T, store *testkit.Store, email string, tier role.Role) uuid.UUID {
	t.Helper()
	held := store.AddUser(t, email, "Maria Perez", testPassword)
	held.Role = tier.String()
	store.Users[held.ID] = held
	return held.ID
}

func TestUsersListsOnlyTheTenantTheCallerStandsIn(t *testing.T) {
	t.Parallel()

	store := testkit.NewStore()
	mine := roledUser(t, store, "maria@example.com", role.Admin)
	theirs := roledUser(t, store, "stranger@example.com", role.Admin)
	acme, globex := uuid.New(), uuid.New()
	resolver := newAuthResolver(store)
	resolver.Tenants = standingTenants{
		standing: map[uuid.UUID]uuid.UUID{mine: acme, theirs: globex},
	}
	client := newGraphClient(t, resolver, mine)

	var listed struct {
		Users []struct {
			Email string `json:"email"`
		} `json:"users"`
	}
	client.MustPost(`{ users { email } }`, &listed)

	if len(listed.Users) != 1 {
		t.Fatalf("users = %d accounts, want only the tenant the caller stands in", len(listed.Users))
	}
	if listed.Users[0].Email != "maria@example.com" {
		t.Errorf("email = %q, want the caller's own account", listed.Users[0].Email)
	}
}

func TestUsersListsTheDefaultTenantWhenNoRowPlacesAnAccount(t *testing.T) {
	t.Parallel()

	store := testkit.NewStore()
	mine := roledUser(t, store, "maria@example.com", role.Admin)
	unplaced := roledUser(t, store, "unplaced@example.com", role.Member)
	theirs := roledUser(t, store, "stranger@example.com", role.Member)
	resolver := newAuthResolver(store)
	resolver.Tenants = standingTenants{standing: map[uuid.UUID]uuid.UUID{theirs: uuid.New()}}
	client := newGraphClient(t, resolver, mine)

	var listed struct {
		Users []struct {
			ID string `json:"id"`
		} `json:"users"`
	}
	client.MustPost(`{ users { id } }`, &listed)

	held := make(map[string]bool, len(listed.Users))
	for _, listedUser := range listed.Users {
		held[listedUser.ID] = true
	}
	if !held[mine.String()] || !held[unplaced.String()] {
		t.Errorf("users = %v, want every account no row places read as the default tenant", held)
	}
	if held[theirs.String()] {
		t.Error("users carried another tenant's account, want it withheld")
	}
}

func TestSetUserDisabledRefusesAnAccountOfAnotherTenant(t *testing.T) {
	t.Parallel()

	store := testkit.NewStore()
	mine := roledUser(t, store, "maria@example.com", role.Admin)
	theirs := roledUser(t, store, "stranger@example.com", role.Admin)
	resolver := newAuthResolver(store)
	resolver.Tenants = standingTenants{
		standing: map[uuid.UUID]uuid.UUID{mine: uuid.New(), theirs: uuid.New()},
	}
	actor := authkit.Identity{ID: mine, Role: role.Admin.String()}
	client := newActingClient(t, resolver, actor)

	answered, err := client.RawPost(settingDisabled(theirs, true))

	if err != nil {
		t.Fatalf("RawPost() error = %v, want nil", err)
	}
	if len(answered.Errors) == 0 {
		t.Error("setUserDisabled answered no error, want an account of another tenant refused")
	}
	if store.Users[theirs].Disabled {
		t.Error("stored account is disabled, want the refused write to leave it")
	}
}

func TestUsersReportsATenantStoreFailure(t *testing.T) {
	t.Parallel()

	store := testkit.NewStore()
	mine := roledUser(t, store, "maria@example.com", role.Admin)
	resolver := newAuthResolver(store)
	resolver.Tenants = failingTenantStore{}
	client := newGraphClient(t, resolver, mine)

	answered, err := client.RawPost(`{ users { id } }`)

	if err != nil {
		t.Fatalf("RawPost() error = %v, want nil", err)
	}
	if len(answered.Errors) == 0 {
		t.Error("users answered no error, want the tenant store failure reported")
	}
}

func TestSetUserDisabledReportsATenantStoreFailure(t *testing.T) {
	t.Parallel()

	store := testkit.NewStore()
	mine := roledUser(t, store, "maria@example.com", role.Admin)
	target := roledUser(t, store, "colleague@example.com", role.Admin)
	resolver := newAuthResolver(store)
	resolver.Tenants = failingTenantStore{}
	actor := authkit.Identity{ID: mine, Role: role.Admin.String()}
	client := newActingClient(t, resolver, actor)

	answered, err := client.RawPost(settingDisabled(target, true))

	if err != nil {
		t.Fatalf("RawPost() error = %v, want nil", err)
	}
	if len(answered.Errors) == 0 {
		t.Error("setUserDisabled answered no error, want the tenant store failure reported")
	}
}

func TestUsersListsEveryAccountWhereNoTenantStoreAnswers(t *testing.T) {
	t.Parallel()

	store := testkit.NewStore()
	mine := roledUser(t, store, "maria@example.com", role.Admin)
	roledUser(t, store, "colleague@example.com", role.Member)
	resolver := newAuthResolver(store)
	client := newGraphClient(t, resolver, mine)

	var listed struct {
		Users []struct {
			ID string `json:"id"`
		} `json:"users"`
	}
	client.MustPost(`{ users { id } }`, &listed)

	if len(listed.Users) != 2 {
		t.Errorf("users = %d accounts, want every account of a single tenant install", len(listed.Users))
	}
}

func TestSetUserDisabledAdmitsAnAccountOfTheCallersOwnTenant(t *testing.T) {
	t.Parallel()

	store := testkit.NewStore()
	mine := roledUser(t, store, "maria@example.com", role.Admin)
	colleague := roledUser(t, store, "colleague@example.com", role.Admin)
	acme := uuid.New()
	resolver := newAuthResolver(store)
	resolver.Tenants = standingTenants{
		standing: map[uuid.UUID]uuid.UUID{mine: acme, colleague: acme},
	}
	actor := authkit.Identity{ID: mine, Role: role.Admin.String()}
	client := newActingClient(t, resolver, actor)

	answered, err := client.RawPost(settingDisabled(colleague, true))

	if err != nil {
		t.Fatalf("RawPost() error = %v, want nil", err)
	}
	if len(answered.Errors) != 0 {
		t.Errorf("setUserDisabled answered %v, want a tenant managing its own accounts", answered.Errors)
	}
	if !store.Users[colleague].Disabled {
		t.Error("stored account is enabled, want the write to land inside the tenant")
	}
}
