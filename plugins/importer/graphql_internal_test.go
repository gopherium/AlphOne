// SPDX-License-Identifier: Elastic-2.0

package importer

import (
	"errors"
	"testing"

	"github.com/gopherium/alphone/sdk"
)

func TestGraphRowLimitBounds(t *testing.T) {
	t.Parallel()

	if got, err := graphRowLimit(nil); err != nil || got != maxRows {
		t.Errorf("graphRowLimit(nil) = %d, %v, want the cap %d", got, err, maxRows)
	}
	top := maxRows
	if got, err := graphRowLimit(&top); err != nil || got != maxRows {
		t.Errorf("graphRowLimit(%d) = %d, %v, want it accepted", maxRows, got, err)
	}
	for _, invalid := range []int{0, -1, maxRows + 1} {
		_, err := graphRowLimit(&invalid)
		var coded sdk.GraphError
		if !errors.As(err, &coded) || coded.Code != "VALIDATION" {
			t.Errorf("graphRowLimit(%d) error = %v, want a VALIDATION graph error", invalid, err)
		}
	}
}

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
