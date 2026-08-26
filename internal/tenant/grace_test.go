// SPDX-License-Identifier: Elastic-2.0

package tenant_test

import (
	"testing"
	"time"

	"github.com/gopherium/alphone/internal/tenant"
)

// fortnight is the grace window the deactivation tests measure against.
const fortnight = 14 * 24 * time.Hour

func TestALiveTenantAcceptsMachineTraffic(t *testing.T) {
	t.Parallel()

	held := tenant.Tenant{Name: "Acme"}

	if !held.AcceptsMachineTraffic(time.Now(), fortnight) {
		t.Error("AcceptsMachineTraffic() = false for a live tenant, want it accepted")
	}
}

func TestALiveTenantAcceptsMachineTrafficUnderAnyWindow(t *testing.T) {
	t.Parallel()

	held := tenant.Tenant{Name: "Acme"}

	if !held.AcceptsMachineTraffic(time.Now(), 0) {
		t.Error("AcceptsMachineTraffic() = false under a zero window, want a live tenant unaffected")
	}
}

func TestADeactivatedTenantAcceptsMachineTrafficInsideTheGraceWindow(t *testing.T) {
	t.Parallel()

	closed := time.Now().Add(-13 * 24 * time.Hour)
	held := tenant.Tenant{Name: "Acme", Deactivated: true, DeactivatedAt: closed}

	if !held.AcceptsMachineTraffic(time.Now(), fortnight) {
		t.Error("AcceptsMachineTraffic() = false inside the window, want the traffic still recorded")
	}
}

func TestADeactivatedTenantRefusesMachineTrafficAfterTheGraceWindow(t *testing.T) {
	t.Parallel()

	closed := time.Now().Add(-15 * 24 * time.Hour)
	held := tenant.Tenant{Name: "Acme", Deactivated: true, DeactivatedAt: closed}

	if held.AcceptsMachineTraffic(time.Now(), fortnight) {
		t.Error("AcceptsMachineTraffic() = true past the window, want every channel stopped")
	}
}

func TestTheGraceWindowEndsExactlyAtItsLength(t *testing.T) {
	t.Parallel()

	closed := time.Now().Add(-fortnight)
	held := tenant.Tenant{Name: "Acme", Deactivated: true, DeactivatedAt: closed}

	if held.AcceptsMachineTraffic(time.Now(), fortnight) {
		t.Error("AcceptsMachineTraffic() = true at the boundary, want the window closed")
	}
}

func TestAZeroGraceWindowStopsADeactivatedTenantAtOnce(t *testing.T) {
	t.Parallel()

	held := tenant.Tenant{Name: "Acme", Deactivated: true, DeactivatedAt: time.Now()}

	if held.AcceptsMachineTraffic(time.Now(), 0) {
		t.Error("AcceptsMachineTraffic() = true under a zero window, want it stopped at once")
	}
}

func TestTheDefaultGraceWindowIsAFortnight(t *testing.T) {
	t.Parallel()

	if tenant.DefaultMachineGrace != fortnight {
		t.Errorf("DefaultMachineGrace = %v, want %v", tenant.DefaultMachineGrace, fortnight)
	}
}
