// SPDX-License-Identifier: Elastic-2.0

package message

import (
	"errors"
	"testing"
)

func TestCappedBuilderWritesUpToItsLimit(t *testing.T) {
	t.Parallel()

	b := &cappedBuilder{limit: 6}

	if n, err := b.Write([]byte("Lamp")); n != 4 || err != nil {
		t.Errorf("Write(%q) = %d, %v, want 4, nil", "Lamp", n, err)
	}
	if n, err := b.Write([]byte("Shade")); n != 0 || !errors.Is(err, errOverCap) {
		t.Errorf("Write(%q) past the limit = %d, %v, want 0, %v", "Shade", n, err, errOverCap)
	}
	if got := b.String(); got != "Lamp" {
		t.Errorf("String() = %q, want %q", got, "Lamp")
	}
}
