// SPDX-License-Identifier: Elastic-2.0

package alphone_test

import (
	"regexp"
	"testing"

	"github.com/gopherium/alphone"
)

func TestVersionIsCleanSemver(t *testing.T) {
	t.Parallel()

	got := alphone.Version()

	if !regexp.MustCompile(`^\d+\.\d+\.\d+$`).MatchString(got) {
		t.Errorf("Version() = %q, want a trimmed semver like 0.1.0", got)
	}
}
