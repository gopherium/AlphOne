// SPDX-License-Identifier: Elastic-2.0

package importer_test

import (
	"slices"
	"testing"

	"github.com/gopherium/alphone/plugins/importer"
	"github.com/gopherium/alphone/sdk"
)

var _ sdk.EventProvider = (*importer.Plugin)(nil)

func TestThePluginDeclaresTheEventItPublishes(t *testing.T) {
	t.Parallel()

	if got := (&importer.Plugin{}).Events(); !slices.Equal(got, []string{"import.completed"}) {
		t.Errorf("Events() = %v, want [import.completed]", got)
	}
}
