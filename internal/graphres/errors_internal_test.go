// SPDX-License-Identifier: Elastic-2.0

package graphres

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/gopherium/gouncer"
	"github.com/gopherium/gouncer/authkit"

	"github.com/gopherium/alphone/internal/contact"
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

// frontendTemplates returns the source of the frontend messages every reason is spoken through.
func frontendTemplates(t *testing.T) string {
	t.Helper()
	templates, err := os.ReadFile(filepath.Join("..", "..", "frontend", "src", "i18n", "errorTemplates.ts"))
	if err != nil {
		t.Fatalf("reading the frontend templates: %v", err)
	}
	return string(templates)
}

func TestEveryCoreReasonHasAFrontendMessage(t *testing.T) {
	t.Parallel()

	templates := frontendTemplates(t)

	for _, held := range reasonsFor {
		if !strings.Contains(templates, "\t"+held.reason+": __(") {
			t.Errorf("reason %q has no frontend message, want the screens to speak it", held.reason)
		}
	}
}

func TestEveryBrickReasonTheUserWritesAnswerHasAFrontendMessage(t *testing.T) {
	t.Parallel()

	templates := frontendTemplates(t)

	for _, sentinel := range []error{
		authkit.ErrRoleChanged,
		authkit.ErrUnknownRole,
		gouncer.ErrUserNotFound,
		gouncer.ErrEmailTaken,
	} {
		_, response, ok := authkit.ErrorResponseForAuthError(sentinel)
		if !ok {
			t.Errorf("%v names no brick reason, want authkit to answer it", sentinel)
			continue
		}
		if !strings.Contains(templates, "\t"+response.Code+": __(") {
			t.Errorf("reason %q has no frontend message, want the screens to speak it", response.Code)
		}
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

func TestReasonOfAnswersNothingForAnUnlistedError(t *testing.T) {
	t.Parallel()

	named, meta := reasonOf(fmt.Errorf("nothing the tables know"))

	if named != "" || meta != nil {
		t.Errorf("reasonOf() = %q, %v, want nothing for an error outside every table", named, meta)
	}
}

func TestABrickSentinelSpeaksItsOwnReason(t *testing.T) {
	t.Parallel()

	presented := PresentError(context.Background(), fmt.Errorf("wrap: %w", gouncer.ErrEmailTaken))

	if got, want := presented.Extensions["reason"], "email_taken"; got != want {
		t.Errorf("reason = %v, want %v, the brick already names its conditions", got, want)
	}
}
