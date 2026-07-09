// SPDX-License-Identifier: Elastic-2.0

package sdk

import (
	"go/build"
	"slices"
	"strings"
	"testing"
)

func TestContractKeepsItsDependenciesMinimal(t *testing.T) {
	t.Parallel()

	pkg, err := build.ImportDir(".", 0)
	if err != nil {
		t.Fatalf("inspecting package: %v", err)
	}

	allowedThirdParty := []string{"github.com/google/uuid"}
	for _, imported := range pkg.Imports {
		if strings.HasPrefix(imported, "github.com/gopherium/alphone") {
			t.Errorf("sdk imports %q; the contract must never depend on the application", imported)
		}
		first, _, _ := strings.Cut(imported, "/")
		if strings.Contains(first, ".") && !slices.Contains(allowedThirdParty, imported) {
			t.Errorf("sdk imports %q; third-party imports are limited to %v", imported, allowedThirdParty)
		}
	}
}
