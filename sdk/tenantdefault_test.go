// SPDX-License-Identifier: Elastic-2.0

package sdk

import (
	"testing"

	"github.com/google/uuid"
)

func TestTheDefaultTenantServesARequestNoHostPlaced(t *testing.T) {
	t.Parallel()

	got := TenantOrDefault(t.Context())

	if got != DefaultTenantID {
		t.Errorf("TenantOrDefault() = %v, want the default tenant", got)
	}
}

func TestTheStandingTenantServesARequestTheHostPlaced(t *testing.T) {
	t.Parallel()

	standing := uuid.Must(uuid.NewV7())

	got := TenantOrDefault(WithTenant(t.Context(), standing))

	if got != standing {
		t.Errorf("TenantOrDefault() = %v, want the standing tenant %v", got, standing)
	}
}
