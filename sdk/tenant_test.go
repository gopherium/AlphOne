// SPDX-License-Identifier: Elastic-2.0

package sdk

import (
	"testing"

	"github.com/google/uuid"
)

func TestTenantRoundTripsThroughTheContext(t *testing.T) {
	t.Parallel()

	standing := uuid.Must(uuid.NewV7())

	ctx := WithTenant(t.Context(), standing)

	got, ok := TenantFromContext(ctx)
	if !ok {
		t.Fatal("TenantFromContext() ok = false, want true after WithTenant")
	}
	if got != standing {
		t.Errorf("TenantFromContext() = %v, want %v", got, standing)
	}
}

func TestTenantIsAbsentWithoutAHost(t *testing.T) {
	t.Parallel()

	got, ok := TenantFromContext(t.Context())

	if ok {
		t.Errorf("TenantFromContext() = %v with ok = true, want absence without a host", got)
	}
}
