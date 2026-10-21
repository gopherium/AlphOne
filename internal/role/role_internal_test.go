// SPDX-License-Identifier: Elastic-2.0

package role

import (
	"errors"
	"testing"

	"github.com/gopherium/framework/goncierge"
)

func TestMustRegistryPanicsOnARoleNameTheRuleRefuses(t *testing.T) {
	t.Parallel()

	defer func() {
		recovered, _ := recover().(error)
		if !errors.Is(recovered, goncierge.ErrInvalidName) {
			t.Fatalf("mustRegistry() recovered %v, want a panic with %v", recovered, goncierge.ErrInvalidName)
		}
	}()

	mustRegistry([]goncierge.Role{{Name: "Admin"}})
}
