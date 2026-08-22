// SPDX-License-Identifier: Elastic-2.0

package role_test

import (
	"errors"
	"slices"
	"testing"

	"github.com/gopherium/alphone/internal/role"
)

func TestAnAdminManagesUsers(t *testing.T) {
	t.Parallel()

	if !role.Can(role.Admin, role.ManageUsers) {
		t.Error("Can(admin, manage_users) = false, want true")
	}
}

func TestAMemberHoldsNoCapability(t *testing.T) {
	t.Parallel()

	if role.Can(role.Member, role.ManageUsers) {
		t.Error("Can(member, manage_users) = true, want false")
	}
	if got := role.CapabilitiesOf(role.Member); got == nil || len(got) != 0 {
		t.Errorf("CapabilitiesOf(member) = %v, want an empty list a plugin can range over", got)
	}
}

func TestARoleTheRegistryDoesNotKnowHoldsNothing(t *testing.T) {
	t.Parallel()

	for _, unknown := range []role.Role{"", "root", "ADMIN", " admin"} {
		if role.Can(unknown, role.ManageUsers) {
			t.Errorf("Can(%q, manage_users) = true, want false", unknown)
		}
		if got := role.CapabilitiesOf(unknown); got == nil || len(got) != 0 {
			t.Errorf("CapabilitiesOf(%q) = %v, want an empty list", unknown, got)
		}
	}
}

func TestTheCoreKnowsOnlyAdminAndMember(t *testing.T) {
	t.Parallel()

	if got := role.Tiers(); !slices.Equal(got, []string{"admin", "member"}) {
		t.Errorf("Tiers() = %v, want admin and member alone, a plugin declares the rest", got)
	}
	if got := role.Privileged(); !slices.Equal(got, []string{"admin"}) {
		t.Errorf("Privileged() = %v, want admin alone", got)
	}
}

func TestTheDefaultRegistryAnswersThePackageFunctions(t *testing.T) {
	t.Parallel()

	if !role.Outranks(role.Admin, role.Member) {
		t.Error("Outranks(admin, member) = false, want true")
	}
	if got := role.Grantable(role.Admin); !slices.Equal(got, []role.Role{role.Admin, role.Member}) {
		t.Errorf("Grantable(admin) = %v, want admin then member", got)
	}
	if err := role.Grant("", role.ManageUsers); !errors.Is(err, role.ErrEmptyRole) {
		t.Errorf("Grant(\"\") error = %v, want ErrEmptyRole", err)
	}
}

func TestAPluginDeclaresARoleWithItsCapabilities(t *testing.T) {
	t.Parallel()

	registry := role.NewRegistry()

	if err := registry.Grant("steward", role.ManageUsers, "manage_reports"); err != nil {
		t.Fatalf("Grant(steward) error = %v, want nil", err)
	}

	if !registry.Can("steward", "manage_reports") {
		t.Error("Can(steward, manage_reports) = false, want the declared capability held")
	}
	if got := registry.Privileged(); !slices.Equal(got, []string{"admin", "steward"}) {
		t.Errorf("Privileged() = %v, want every role managing users, in stored order", got)
	}
	if got := registry.Roles(); !slices.Equal(got, []role.Role{"admin", "member", "steward"}) {
		t.Errorf("Roles() = %v, want the declared role beside the core ones", got)
	}
	if parsed, err := registry.Parse("steward"); err != nil || parsed != "steward" {
		t.Errorf("Parse(steward) = %q, %v, want the declared role accepted", parsed, err)
	}
}

func TestAPluginWidensACoreRole(t *testing.T) {
	t.Parallel()

	registry := role.NewRegistry()

	if err := registry.Grant(role.Admin, "manage_reports"); err != nil {
		t.Fatalf("Grant(admin) error = %v, want nil", err)
	}

	if !registry.Can(role.Admin, "manage_reports") {
		t.Error("Can(admin, manage_reports) = false, want the added capability held")
	}
	if !registry.Can(role.Admin, role.ManageUsers) {
		t.Error("Can(admin, manage_users) = false, want the core capability kept")
	}
	if got := registry.CapabilitiesOf(role.Admin); !slices.Equal(got, []string{"manage_users", "manage_reports"}) {
		t.Errorf("CapabilitiesOf(admin) = %v, want the core capability then the added one", got)
	}
}

func TestGrantingTwiceHoldsEachCapabilityOnce(t *testing.T) {
	t.Parallel()

	registry := role.NewRegistry()

	if err := registry.Grant("steward", "manage_reports"); err != nil {
		t.Fatalf("first Grant() error = %v, want nil", err)
	}
	if err := registry.Grant("steward", "manage_reports", "manage_users"); err != nil {
		t.Fatalf("second Grant() error = %v, want nil", err)
	}

	if got := registry.CapabilitiesOf("steward"); !slices.Equal(got, []string{"manage_reports", "manage_users"}) {
		t.Errorf("CapabilitiesOf(steward) = %v, want each capability once in the order granted", got)
	}
}

func TestGrantRefusesARoleWithNoName(t *testing.T) {
	t.Parallel()

	registry := role.NewRegistry()

	err := registry.Grant("", role.ManageUsers)

	if !errors.Is(err, role.ErrEmptyRole) {
		t.Errorf("Grant(\"\") error = %v, want ErrEmptyRole", err)
	}
	if got := registry.Roles(); !slices.Equal(got, []role.Role{"admin", "member"}) {
		t.Errorf("Roles() = %v, want the refused grant to declare nothing", got)
	}
}

func TestOutranksHoldsEveryCapabilityOfTheTarget(t *testing.T) {
	t.Parallel()

	registry := role.NewRegistry()
	if err := registry.Grant("steward", role.ManageUsers, "manage_reports"); err != nil {
		t.Fatalf("Grant() error = %v, want nil", err)
	}

	for _, held := range []struct {
		caller, target role.Role
		want           bool
	}{
		{"steward", "steward", true},
		{"steward", role.Admin, true},
		{"steward", role.Member, true},
		{role.Admin, "steward", false},
		{role.Admin, role.Admin, true},
		{role.Admin, role.Member, true},
		{role.Member, role.Admin, false},
		{role.Member, role.Member, true},
		{"root", role.Member, true},
		{role.Member, "root", true},
	} {
		if got := registry.Outranks(held.caller, held.target); got != held.want {
			t.Errorf("Outranks(%q, %q) = %v, want %v", held.caller, held.target, got, held.want)
		}
	}
}

func TestGrantableListsTheRolesTheCallerOutranksWidestFirst(t *testing.T) {
	t.Parallel()

	registry := role.NewRegistry()
	if err := registry.Grant("steward", role.ManageUsers, "manage_reports"); err != nil {
		t.Fatalf("Grant() error = %v, want nil", err)
	}

	for _, held := range []struct {
		caller role.Role
		want   []role.Role
	}{
		{"steward", []role.Role{"steward", role.Admin, role.Member}},
		{role.Admin, []role.Role{role.Admin, role.Member}},
		{role.Member, []role.Role{role.Member}},
		{"root", []role.Role{role.Member}},
	} {
		if got := registry.Grantable(held.caller); !slices.Equal(got, held.want) {
			t.Errorf("Grantable(%q) = %v, want %v", held.caller, got, held.want)
		}
	}
}

func TestRolesHoldingTheSameCountOrderByName(t *testing.T) {
	t.Parallel()

	registry := role.NewRegistry()
	if err := registry.Grant("auditor", "read_reports"); err != nil {
		t.Fatalf("Grant() error = %v, want nil", err)
	}

	got := registry.Grantable(role.Admin)

	if !slices.Equal(got, []role.Role{role.Admin, role.Member}) {
		t.Errorf("Grantable(admin) = %v, want an admin unable to grant a capability it lacks", got)
	}
	if got := registry.Grantable("auditor"); !slices.Equal(got, []role.Role{"auditor", role.Member}) {
		t.Errorf("Grantable(auditor) = %v, want its own role before the one holding less", got)
	}
}
