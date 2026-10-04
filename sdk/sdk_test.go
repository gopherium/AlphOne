// SPDX-License-Identifier: Elastic-2.0

package sdk

import (
	"reflect"
	"testing"

	"github.com/gopherium/framework/pluginkit"
)

func TestSeederIsTheHostSeederContract(t *testing.T) {
	t.Parallel()

	named, hosted := reflect.TypeFor[Seeder](), reflect.TypeFor[pluginkit.Seeder]()

	if named != hosted {
		t.Errorf("Seeder = %v, want the host's own %v", named, hosted)
	}
}
