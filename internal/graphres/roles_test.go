// SPDX-License-Identifier: Elastic-2.0

package graphres_test

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"

	"github.com/gopherium/gouncer/authkit/testkit"

	"github.com/gopherium/alphone/internal/graphres"
	"github.com/gopherium/alphone/internal/role"
)

// errRoleBackend reports a role store that cannot answer.
var errRoleBackend = errors.New("role backend unavailable")

// failingRoleStore refuses every read.
type failingRoleStore struct{}

// RoleOf refuses to answer one tier.
func (failingRoleStore) RoleOf(context.Context, uuid.UUID) (role.Role, error) {
	return role.Member, errRoleBackend
}

// RolesOf refuses to answer many tiers.
func (failingRoleStore) RolesOf(context.Context, []uuid.UUID) (map[uuid.UUID]role.Role, error) {
	return nil, errRoleBackend
}

// standingRoleStore answers one fixed tier for everybody.
type standingRoleStore struct {
	tier role.Role
}

// RoleOf answers the fixed tier.
func (s standingRoleStore) RoleOf(context.Context, uuid.UUID) (role.Role, error) {
	return s.tier, nil
}

// RolesOf answers the fixed tier for each named user.
func (s standingRoleStore) RolesOf(_ context.Context, ids []uuid.UUID) (map[uuid.UUID]role.Role, error) {
	tiers := make(map[uuid.UUID]role.Role, len(ids))
	for _, id := range ids {
		tiers[id] = s.tier
	}
	return tiers, nil
}

// newRoledResolver returns an auth resolver holding one account whose tiers come from roles.
func newRoledResolver(t *testing.T, roles graphres.RoleStore) *graphres.Resolver {
	t.Helper()
	store := testkit.NewStore()
	store.AddUser(t, "grace@example.com", "Grace Hopper", testPassword)
	resolver := newAuthResolver(store)
	resolver.Roles = roles
	return resolver
}

func TestUsersReportsAFailingRoleStore(t *testing.T) {
	t.Parallel()

	resolver := newRoledResolver(t, failingRoleStore{})
	client := newGraphClient(t, resolver, uuid.Must(uuid.NewV7()))

	answered, err := client.RawPost(`{ users { id role } }`)

	if err != nil {
		t.Fatalf("RawPost() error = %v, want nil", err)
	}
	if len(answered.Errors) == 0 {
		t.Error("users answered no error while the role store refuses, want one")
	}
}

func TestUsersCarriesEachAccountsTier(t *testing.T) {
	t.Parallel()

	resolver := newRoledResolver(t, standingRoleStore{tier: role.Admin})
	client := newGraphClient(t, resolver, uuid.Must(uuid.NewV7()))

	var listed struct {
		Users []struct {
			Role string `json:"role"`
		} `json:"users"`
	}
	client.MustPost(`{ users { role } }`, &listed)

	if len(listed.Users) == 0 {
		t.Fatal("users answered nothing, want the seeded account")
	}
	if listed.Users[0].Role != role.Admin.String() {
		t.Errorf("role = %q, want %q", listed.Users[0].Role, role.Admin.String())
	}
}

func TestUsersLeavesEverybodyAMemberWithNoStoreWired(t *testing.T) {
	t.Parallel()

	resolver := newRoledResolver(t, nil)
	client := newGraphClient(t, resolver, uuid.Must(uuid.NewV7()))

	var listed struct {
		Users []struct {
			Role string `json:"role"`
		} `json:"users"`
	}
	client.MustPost(`{ users { role } }`, &listed)

	if len(listed.Users) == 0 {
		t.Fatal("users answered nothing, want the seeded account")
	}
	if listed.Users[0].Role != role.Member.String() {
		t.Errorf("role = %q, want %q with no store wired", listed.Users[0].Role, role.Member.String())
	}
}

func TestLoginReportsAFailingRoleStore(t *testing.T) {
	t.Parallel()

	resolver := newRoledResolver(t, failingRoleStore{})
	client := newHTTPGraphClient(t, resolver, uuid.Nil)

	answered, err := client.RawPost(
		`mutation { login(email: "grace@example.com", password: "` + testPassword + `") { me { role } } }`)

	if err != nil {
		t.Fatalf("RawPost() error = %v, want nil", err)
	}
	if len(answered.Errors) == 0 {
		t.Error("login answered no error while the role store refuses, want one")
	}
}

func TestLoginAnswersTheCallersTier(t *testing.T) {
	t.Parallel()

	resolver := newRoledResolver(t, standingRoleStore{tier: role.Admin})
	client := newHTTPGraphClient(t, resolver, uuid.Nil)

	var answered struct {
		Login struct {
			Me struct {
				Role string `json:"role"`
			} `json:"me"`
		} `json:"login"`
	}
	client.MustPost(
		`mutation { login(email: "grace@example.com", password: "`+testPassword+`") { me { role } } }`, &answered)

	if answered.Login.Me.Role != role.Admin.String() {
		t.Errorf("role = %q, want %q", answered.Login.Me.Role, role.Admin.String())
	}
}
