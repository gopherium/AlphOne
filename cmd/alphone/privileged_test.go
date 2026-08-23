// SPDX-License-Identifier: Elastic-2.0

package main

import (
	"slices"
	"testing"

	"github.com/gopherium/alphone/internal/role"
)

func TestTheAdminSeamCarriesThePrivilegedCover(t *testing.T) {
	t.Parallel()

	held := adminConfig(nil)

	if !slices.Equal(held.Privileged, role.Privileged()) {
		t.Errorf("Privileged = %v, want %v, an empty cover admits every role at the brick's guard",
			held.Privileged, role.Privileged())
	}
	if len(held.Privileged) == 0 {
		t.Error("the cover is empty, want the roles that administer accounts")
	}
}

func TestTheLoginSeamCarriesThePrivilegedCover(t *testing.T) {
	t.Parallel()

	held := authConfig(nil)

	if !slices.Equal(held.Privileged, role.Privileged()) {
		t.Errorf("Privileged = %v, want %v", held.Privileged, role.Privileged())
	}
	if held.CookieName == "" {
		t.Error("the login seam names no cookie, want the session cookie the server reads")
	}
}
