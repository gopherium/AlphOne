// SPDX-License-Identifier: Elastic-2.0

package importer

import (
	"testing"
)

func TestToGraphMappingOrdersAndSkipsMalformedKeys(t *testing.T) {
	t.Parallel()

	listed := toGraphMapping(mapping{"2": fieldEmail, "0": fieldContactName, "x": fieldPhone})

	if len(listed) != 2 {
		t.Fatalf("assignments = %d, want the malformed key skipped", len(listed))
	}
	if listed[0].Column != 0 || listed[0].Field != "name" || listed[1].Column != 2 || listed[1].Field != "email" {
		t.Errorf("assignments = %+v, want name then email in column order", listed)
	}
}
