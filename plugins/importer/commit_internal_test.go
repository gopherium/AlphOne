// SPDX-License-Identifier: Elastic-2.0

package importer

import (
	"errors"
	"testing"

	"github.com/gopherium/alphone/sdk"
)

func TestCellAtAnswersNothingOutsideTheRow(t *testing.T) {
	t.Parallel()

	if got := cellAt([]string{"a", "b"}, "5"); got != "" {
		t.Errorf("cellAt(5) = %q, want nothing past the row", got)
	}
	if got := cellAt([]string{"a", "b"}, "not a number"); got != "" {
		t.Errorf("cellAt(text) = %q, want nothing for an unreadable index", got)
	}
	if got := cellAt([]string{"a", "b"}, "1"); got != "b" {
		t.Errorf("cellAt(1) = %q, want the named cell", got)
	}
}

func TestClassifyClaimErrorNamesTheReason(t *testing.T) {
	t.Parallel()

	classified := classifyClaimError(errNoMapping)

	var raised sdk.GraphError
	if !errors.As(classified, &raised) || raised.Reason != "mapping_required" {
		t.Errorf("error = %v, want the missing mapping named as a reason", classified)
	}
}
