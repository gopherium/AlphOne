// SPDX-License-Identifier: Elastic-2.0

package main

import (
	"slices"
	"testing"

	"github.com/gopherium/alphone/internal/role"
)

func TestTheAdminSeamCarriesThePrivilegedCover(t *testing.T) {
	t.Parallel()

	registry := role.NewRegistry()

	held := adminConfig(composed{roles: registry})

	if want := registry.HoldersOf(role.ManageUsers); !slices.Equal(held.Privileged, want) {
		t.Errorf("Privileged = %v, want %v, an empty cover admits every role at the brick's guard",
			held.Privileged, want)
	}
	if len(held.Privileged) == 0 {
		t.Error("the cover is empty, want the roles that administer accounts")
	}
	if held.Roles != registry {
		t.Errorf("Roles = %v, want the registry handed in", held.Roles)
	}
}

func TestTheLoginSeamCarriesThePrivilegedCover(t *testing.T) {
	t.Parallel()

	registry := role.NewRegistry()

	held := authConfig(composed{roles: registry})

	if want := registry.HoldersOf(role.ManageUsers); !slices.Equal(held.Privileged, want) {
		t.Errorf("Privileged = %v, want %v", held.Privileged, want)
	}
	if len(held.Privileged) == 0 {
		t.Error("the cover is empty, want the roles that administer accounts")
	}
	if held.Capabilities != registry {
		t.Errorf("Capabilities = %v, want the registry handed in", held.Capabilities)
	}
	if held.CookieName == "" {
		t.Error("the login seam names no cookie, want the session cookie the server reads")
	}
}
