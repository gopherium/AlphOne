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

// ownedRegistry returns the core registry beside an owner role the owners source declares with the core capabilities.
func ownedRegistry(t *testing.T) *goncierge.Registry {
	t.Helper()
	registry := role.NewRegistry()
	owner := []goncierge.Role{{Name: "owner", Capabilities: []string{role.ManageUsers, role.ManageWebhooks}}}
	if err := role.Declare(registry, "owners", owner); err != nil {
		t.Fatalf("Declare(owners) error = %v, want nil", err)
	}
	return registry
}

// widenedAdmin is a declaration giving the admin role a capability of the reports source.
var widenedAdmin = []goncierge.Role{{Name: role.Admin.String(), Capabilities: []string{"reports.export_reports"}}}

func TestDeclareWithTopRoleGivesTheTopRoleWhatTheSourceGivesAdmin(t *testing.T) {
	t.Parallel()

	registry := ownedRegistry(t)

	if err := role.DeclareWithTopRole(registry, "owner", "reports", widenedAdmin); err != nil {
		t.Fatalf("DeclareWithTopRole() error = %v, want nil", err)
	}
	if !registry.Can("owner", "reports.export_reports") {
		t.Error("Can(owner, reports.export_reports) = false, want the top role to hold what the source gives admin")
	}
	if !registry.Outranks("owner", role.Admin.String()) {
		t.Error("Outranks(owner, admin) = false, want the top role to hold every capability admin holds")
	}
}

func TestDeclareWithTopRoleRefusesATopRoleNoOtherSourceCreated(t *testing.T) {
	t.Parallel()

	registry := role.NewRegistry()

	err := role.DeclareWithTopRole(registry, "owner", "reports", widenedAdmin)

	if !errors.Is(err, goncierge.ErrUnknownAdmin) {
		t.Errorf("DeclareWithTopRole() error = %v, want %v", err, goncierge.ErrUnknownAdmin)
	}
	if registry.Can(role.Admin.String(), "reports.export_reports") {
		t.Error("Can(admin, reports.export_reports) = true, want the refused declaration left out")
	}
}

func TestDeclareWithNoTopRoleKeepsTheSourceCapabilityOffEveryOtherRole(t *testing.T) {
	t.Parallel()

	registry := ownedRegistry(t)
	auditor := goncierge.Role{Name: "auditor", Capabilities: []string{"reports.view_reports"}}
	declared := append(slices.Clone(widenedAdmin), auditor)

	if err := role.DeclareWithTopRole(registry, "", "reports", declared); err != nil {
		t.Fatalf("DeclareWithTopRole() error = %v, want nil", err)
	}
	if got := registry.HoldersOf("reports.export_reports"); !slices.Equal(got, []string{"admin"}) {
		t.Errorf("HoldersOf(reports.export_reports) = %v, want admin alone", got)
	}
	if got := registry.HoldersOf("reports.view_reports"); !slices.Equal(got, []string{"auditor"}) {
		t.Errorf("HoldersOf(reports.view_reports) = %v, want auditor alone", got)
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
