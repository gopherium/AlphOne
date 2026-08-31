// SPDX-License-Identifier: Elastic-2.0

package mail

import "testing"

func TestMustSubRejectsInvalidDir(t *testing.T) {
	t.Parallel()

	defer func() {
		if recover() == nil {
			t.Fatal("mustSub(..) did not panic, want a panic")
		}
	}()

	mustSub(embedded, "..")
}
