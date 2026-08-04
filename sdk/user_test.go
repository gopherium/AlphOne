// SPDX-License-Identifier: Elastic-2.0

package sdk

import (
	"testing"

	"github.com/google/uuid"
)

func TestUserRoundTripsThroughTheContext(t *testing.T) {
	t.Parallel()

	acting := uuid.Must(uuid.NewV7())

	ctx := WithUser(t.Context(), acting)

	got, ok := UserFromContext(ctx)
	if !ok {
		t.Fatal("UserFromContext() ok = false, want true after WithUser")
	}
	if got != acting {
		t.Errorf("UserFromContext() = %v, want %v", got, acting)
	}
}

func TestUserIsAbsentWithoutAHost(t *testing.T) {
	t.Parallel()

	got, ok := UserFromContext(t.Context())

	if ok {
		t.Error("UserFromContext() ok = true, want false without a host-provided user")
	}
	if got != uuid.Nil {
		t.Errorf("UserFromContext() = %v, want uuid.Nil", got)
	}
}
