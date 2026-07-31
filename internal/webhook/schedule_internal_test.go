// SPDX-License-Identifier: Elastic-2.0

package webhook

import (
	"testing"
	"time"
)

func TestBackoffScheduleIsBoundedAndDocumented(t *testing.T) {
	t.Parallel()

	created := time.Time{}
	var age time.Duration
	attempts := 0
	for !Exhausted(created, created.Add(age)) {
		attempts++
		wait := Backoff(attempts)
		if wait <= 0 || wait > maxBackoff {
			t.Fatalf("Backoff(%d) = %v, want within (0, %v]", attempts, wait, maxBackoff)
		}
		age += wait
		t.Logf("attempt %d, next retry %v after the event", attempts, age)
	}

	if age < RetryWindow {
		t.Errorf("given up %v after the event, want at least %v", age, RetryWindow)
	}
	if age >= RetryWindow+maxBackoff {
		t.Errorf("given up %v after the event, want under %v", age, RetryWindow+maxBackoff)
	}
}
