// SPDX-License-Identifier: Elastic-2.0

package credential_test

import (
	"testing"

	"github.com/gopherium/alphone/internal/credential"
)

func TestOriginRoundTrip(t *testing.T) {
	t.Parallel()

	ctx := credential.WithTokenOrigin(t.Context(), "n8n production")

	if got, want := credential.Origin(ctx), "token:n8n production"; got != want {
		t.Errorf("Origin() = %q, want %q", got, want)
	}
	if got := credential.Origin(t.Context()); got != "" {
		t.Errorf("Origin(bare context) = %q, want empty", got)
	}
}
