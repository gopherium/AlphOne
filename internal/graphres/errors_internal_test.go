// SPDX-License-Identifier: Elastic-2.0

package graphres

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/gopherium/gouncer"
	"github.com/gopherium/gouncer/authkit"

	"github.com/gopherium/alphone/internal/contact"
	"github.com/gopherium/alphone/internal/role"
	"github.com/gopherium/alphone/sdk"
)

func TestEveryRefusedSentinelNamesAReason(t *testing.T) {
	t.Parallel()

	sentinels := append(append([]error{}, validationErrors...), notFoundErrors...)
	for _, sentinel := range sentinels {
		presented := PresentError(context.Background(), fmt.Errorf("wrap: %w", sentinel))
		named, _ := presented.Extensions["reason"].(string)
		if named == "" {
			t.Errorf("%v names no reason, want every refused sentinel naming one", sentinel)
		}
	}
}

func TestBothLastPrivilegedSentinelsShareOneReason(t *testing.T) {
	t.Parallel()

	core := PresentError(context.Background(), fmt.Errorf("wrap: %w", role.ErrLastAdmin))
	brick := PresentError(context.Background(), fmt.Errorf("wrap: %w", gouncer.ErrLastPrivileged))

	if core.Extensions["reason"] != brick.Extensions["reason"] {
		t.Errorf("reasons %v and %v differ, want one condition named once",
			core.Extensions["reason"], brick.Extensions["reason"])
	}
}

func TestEverySpecialPathNamesItsReason(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name   string
		err    error
		reason string
	}{
		{"identity taken", contact.IdentityExistsError{OwnerID: uuid.Nil}, "identity_taken"},
		{"rate limited", rateLimitedError{retryAfter: 30 * time.Second}, "rate_limited"},
		{"bad credentials", authkit.ErrInvalidCredentials, "credentials_invalid"},
	}
	for _, tc := range cases {
		presented := PresentError(context.Background(), fmt.Errorf("wrap: %w", tc.err))
		if got := presented.Extensions["reason"]; got != tc.reason {
			t.Errorf("%s reason = %v, want %q", tc.name, got, tc.reason)
		}
	}
}

func TestAPluginErrorCarriesItsOwnReason(t *testing.T) {
	t.Parallel()

	raised := sdk.GraphError{
		Code:   "VALIDATION",
		Reason: "field_name_malformed",
		Meta:   map[string]any{"name": "Not CamelCase"},
		Err:    fmt.Errorf("fields: a name is camelCase"),
	}

	presented := PresentError(context.Background(), raised)

	if got := presented.Extensions["reason"]; got != "field_name_malformed" {
		t.Errorf("reason = %v, want the plugin's own reason carried through", got)
	}
	meta, _ := presented.Extensions["meta"].(map[string]any)
	if meta["name"] != "Not CamelCase" {
		t.Errorf("meta = %v, want the plugin's data beside the reason", meta)
	}
}

func TestABrickSentinelSpeaksItsOwnReason(t *testing.T) {
	t.Parallel()

	presented := PresentError(context.Background(), fmt.Errorf("wrap: %w", gouncer.ErrEmailTaken))

	if got, want := presented.Extensions["reason"], "email_taken"; got != want {
		t.Errorf("reason = %v, want %v, the brick already names its conditions", got, want)
	}
}
