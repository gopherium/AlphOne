// SPDX-License-Identifier: Elastic-2.0

package webhook

import (
	"testing"
	"time"
)

func TestBackoffScheduleIsBoundedAndDocumented(t *testing.T) {
	t.Parallel()

	var total time.Duration
	for attempt := 1; attempt <= MaxAttempts; attempt++ {
		total += Backoff(attempt)
		t.Logf("attempt %d waits %v, cumulative %v", attempt, Backoff(attempt), total)
	}

	if total > 3*time.Hour {
		t.Errorf("a spent budget spans %v, want a subscriber given up on within a few hours", total)
	}
}
