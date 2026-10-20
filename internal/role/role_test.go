// SPDX-License-Identifier: Elastic-2.0

package role_test

import (
	"errors"
	"slices"
	"testing"

	"github.com/gopherium/framework/goncierge"

	"github.com/gopherium/alphone/internal/role"
)

func TestTheCoreKnowsOnlyAdminAndMember(t *testing.T) {
	t.Parallel()

	if got := role.NewRegistry().Roles(); !slices.Equal(got, []string{"admin", "member"}) {
		t.Errorf("Roles() = %v, want admin and member alone, a plugin declares the rest", got)
	}
}

func TestOnlyTheAdminManagesUsers(t *testing.T) {
	t.Parallel()

	if got := role.NewRegistry().HoldersOf(role.ManageUsers); !slices.Equal(got, []string{"admin"}) {
		t.Errorf("HoldersOf(manage_users) = %v, want admin alone", got)
	}
}

func TestAnAdminManagesUsersAndWebhooks(t *testing.T) {
	t.Parallel()

	got := role.NewRegistry().CapabilitiesOf(role.Admin.String())

	if !slices.Equal(got, []string{"manage_users", "manage_webhooks"}) {
		t.Errorf("CapabilitiesOf(admin) = %v, want manage_users then manage_webhooks", got)
	}
}

func TestAMemberHoldsNoCapability(t *testing.T) {
	t.Parallel()

	if got := role.NewRegistry().CapabilitiesOf(role.Member.String()); len(got) != 0 {
		t.Errorf("CapabilitiesOf(member) = %v, want an empty list", got)
	}
}

func TestTheCoreGrantsUnderItsOwnSource(t *testing.T) {
	t.Parallel()

	registry := role.NewRegistry()
	registry.Withdraw(role.Source)

	if got := registry.Roles(); len(got) != 0 {
		t.Errorf("Roles() after Withdraw(%q) = %v, want none, the core grants every core role", role.Source, got)
	}
}

func TestDeclareKeepsAPluginCapabilityOffTheAdminRole(t *testing.T) {
	t.Parallel()

	registry := role.NewRegistry()
	declared := []goncierge.Role{{Name: "steward", Capabilities: []string{"steward.manage_reports"}}}

	if err := role.Declare(registry, "steward", declared); err != nil {
		t.Fatalf("Declare() error = %v, want nil", err)
	}
	if !registry.Can("steward", "steward.manage_reports") {
		t.Error("Can(steward, steward.manage_reports) = false, want the declared capability held")
	}
	if registry.Can(role.Admin.String(), "steward.manage_reports") {
		t.Error("Can(admin, steward.manage_reports) = true, want a plugin capability kept off the admin role")
	}
}

func TestDeclareRefusesANewCapabilityOutsideThePluginPrefix(t *testing.T) {
	t.Parallel()

	registry := role.NewRegistry()
	declared := []goncierge.Role{{Name: "steward", Capabilities: []string{"manage_reports"}}}

	if err := role.Declare(registry, "steward", declared); !errors.Is(err, goncierge.ErrOutsidePrefix) {
		t.Errorf("Declare() error = %v, want %v", err, goncierge.ErrOutsidePrefix)
	}
}

func TestDeclareLetsAPluginGrantACoreCapability(t *testing.T) {
	t.Parallel()

	registry := role.NewRegistry()
	declared := []goncierge.Role{{Name: "steward", Capabilities: []string{role.ManageUsers}}}

	if err := role.Declare(registry, "steward", declared); err != nil {
		t.Fatalf("Declare() error = %v, want nil", err)
	}
	if got := registry.HoldersOf(role.ManageUsers); !slices.Equal(got, []string{"admin", "steward"}) {
		t.Errorf("HoldersOf(manage_users) = %v, want admin and steward", got)
	}
}
