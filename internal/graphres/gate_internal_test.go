// SPDX-License-Identifier: Elastic-2.0

package graphres

import "testing"

func TestOnlyLoginFieldsRejectsAnEmptySelection(t *testing.T) {
	t.Parallel()

	if onlyLoginFields(nil) {
		t.Error("onlyLoginFields(nil) = true, want false")
	}
}
