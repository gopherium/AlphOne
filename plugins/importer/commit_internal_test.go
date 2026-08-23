// SPDX-License-Identifier: Elastic-2.0

package importer

import (
	"errors"
	"testing"

	"github.com/gopherium/alphone/sdk"
)

func TestClassifyClaimErrorNamesTheReason(t *testing.T) {
	t.Parallel()

	classified := classifyClaimError(errNoMapping)

	var raised sdk.GraphError
	if !errors.As(classified, &raised) || raised.Reason != "mapping_required" {
		t.Errorf("error = %v, want the missing mapping named as a reason", classified)
	}
}
