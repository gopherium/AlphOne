// SPDX-License-Identifier: Elastic-2.0

package message

import (
	"strings"
	"testing"
)

func TestCutOnLetterCutsAStringWithNoCopy(t *testing.T) {
	text := "aé" + strings.Repeat("b", 64)
	var cut string

	allocations := testing.AllocsPerRun(10, func() { cut = cutOnLetter(text, 2) })

	if cut != "a" || allocations != 0 {
		t.Errorf("cutOnLetter(%q, 2) = %q with %v allocations, want %q with none", text, cut, allocations, "a")
	}
}
