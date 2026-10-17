// SPDX-License-Identifier: Elastic-2.0

package graphres

import (
	"errors"
	"testing"

	"github.com/gopherium/alphone/internal/task"
)

func TestMustClockOfPanicsOnATimeTheDomainRefuses(t *testing.T) {
	t.Parallel()

	refused := "9:00"
	defer func() {
		recovered, _ := recover().(error)
		if !errors.Is(recovered, task.ErrInvalidClock) {
			t.Fatalf("mustClockOf(%q) panicked with %v, want a panic carrying ErrInvalidClock", refused, recovered)
		}
	}()

	mustClockOf(&refused)
}
