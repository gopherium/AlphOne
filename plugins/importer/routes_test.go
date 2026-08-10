// SPDX-License-Identifier: Elastic-2.0

package importer_test

import (
	"testing"

	"github.com/gopherium/alphone/sdk"
)

func TestTheImporterServesNothingOverHTTP(t *testing.T) {
	t.Parallel()

	p := newPlugin(t, unreachableDatabaseURL)

	if _, ok := any(p).(sdk.RouteProvider); ok {
		t.Error("the importer declares HTTP routes, want the graph to be its only surface")
	}
}
