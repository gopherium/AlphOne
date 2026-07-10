// SPDX-License-Identifier: Elastic-2.0

package whatsapp_test

import (
	"testing"

	"github.com/google/go-cmp/cmp"

	"github.com/gopherium/alphone/plugins/whatsapp"
	"github.com/gopherium/alphone/sdk"
)

var _ sdk.PublicPathProvider = (*whatsapp.Plugin)(nil)

func TestPublicPathsDeclareOnlyTheWebhook(t *testing.T) {
	t.Parallel()

	p := newPlugin(t, unreachableDatabaseURL, nil, nil)

	if diff := cmp.Diff([]string{"/webhook"}, p.PublicPaths()); diff != "" {
		t.Errorf("PublicPaths() mismatch (-want +got):\n%s", diff)
	}
}
