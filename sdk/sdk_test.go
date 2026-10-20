// SPDX-License-Identifier: Elastic-2.0

package sdk

import (
	"reflect"
	"testing"

	"github.com/gopherium/framework/goncierge"
	"github.com/gopherium/framework/pluginkit"
)

func TestSeederIsTheHostSeederContract(t *testing.T) {
	t.Parallel()

	named, hosted := reflect.TypeFor[Seeder](), reflect.TypeFor[pluginkit.Seeder]()

	if named != hosted {
		t.Errorf("Seeder = %v, want the host's own %v", named, hosted)
	}
}

func TestRoleDeclarationIsTheRegistrysRole(t *testing.T) {
	t.Parallel()

	named, registered := reflect.TypeFor[RoleDeclaration](), reflect.TypeFor[goncierge.Role]()

	if named != registered {
		t.Errorf("RoleDeclaration = %v, want the registry's own %v", named, registered)
	}
}
