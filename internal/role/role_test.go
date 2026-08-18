// SPDX-License-Identifier: Elastic-2.0

package role_test

import (
	"testing"

	"github.com/gopherium/alphone/internal/role"
)

func TestAdminReachesEveryField(t *testing.T) {
	t.Parallel()

	if !role.Admin.Allows(true) {
		t.Error("Admin.Allows(admin only) = false, want true")
	}
	if !role.Admin.Allows(false) {
		t.Error("Admin.Allows(open) = false, want true")
	}
}

func TestMemberIsRefusedAnAdminField(t *testing.T) {
	t.Parallel()

	if role.Member.Allows(true) {
		t.Error("Member.Allows(admin only) = true, want false")
	}
	if !role.Member.Allows(false) {
		t.Error("Member.Allows(open) = false, want true")
	}
}

func TestOfReadsTheStoredTier(t *testing.T) {
	t.Parallel()

	if got := role.Of("admin"); got != role.Admin {
		t.Errorf("Of(admin) = %v, want %v", got, role.Admin)
	}
	if got := role.Of("member"); got != role.Member {
		t.Errorf("Of(member) = %v, want %v", got, role.Member)
	}
}

func TestOfDemotesAnythingItCannotRead(t *testing.T) {
	t.Parallel()

	for _, stored := range []string{"", "root", "ADMIN", " admin"} {
		if got := role.Of(stored); got != role.Member {
			t.Errorf("Of(%q) = %v, want %v, an unreadable tier demotes", stored, got, role.Member)
		}
	}
}

func TestStringRoundTripsTheStoredForm(t *testing.T) {
	t.Parallel()

	for _, tier := range []role.Role{role.Admin, role.Member} {
		if got := role.Of(tier.String()); got != tier {
			t.Errorf("Of(%q) = %v, want %v", tier.String(), got, tier)
		}
	}
}

func TestTiersNamesEveryStorableTier(t *testing.T) {
	t.Parallel()

	tiers := role.Tiers()

	if len(tiers) != 2 {
		t.Fatalf("Tiers() = %v, want two tiers", tiers)
	}
	for _, tier := range tiers {
		if role.Of(tier) == role.Member && tier != role.Member.String() {
			t.Errorf("Tiers() names %q, which does not read back", tier)
		}
	}
}
