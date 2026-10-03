// SPDX-License-Identifier: Elastic-2.0

package importer_test

import (
	"testing"
	"time"
)

// silentDatabaseURL names a database address that never answers a connection attempt.
const silentDatabaseURL = "postgres://postgres@192.0.2.1:5432/none?sslmode=disable&connect_timeout=5"

// registerBudget is the longest Register may take when it opens nothing.
const registerBudget = time.Second

func TestRegisterOpensNothingOverAnUnreachableDatabase(t *testing.T) {
	t.Parallel()

	started := time.Now()
	newPlugin(t, silentDatabaseURL)

	if took := time.Since(started); took > registerBudget {
		t.Errorf("Register() took %v, want it back within %v having opened nothing", took, registerBudget)
	}
}
