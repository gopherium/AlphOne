// SPDX-License-Identifier: Elastic-2.0

package fields_test

import (
	"context"
	"testing"
	"time"

	"github.com/gopherium/alphone/plugins/fields"
	"github.com/gopherium/alphone/sdk"
)

// silentDatabaseURL names a database address that never answers a connection attempt.
const silentDatabaseURL = "postgres://postgres@192.0.2.1:5432/none?sslmode=disable&connect_timeout=5"

// registerBudget is the longest Register may take when it opens nothing.
const registerBudget = time.Second

func TestRegisterOpensNothingOverAnUnreachableDatabase(t *testing.T) {
	t.Parallel()

	started := time.Now()
	p, err := fields.Register(sdk.Deps{DatabaseURL: silentDatabaseURL})
	took := time.Since(started)

	if err != nil {
		t.Fatalf("Register() error = %v, want nil", err)
	}
	t.Cleanup(func() { _ = p.Stop(context.Background()) })
	if took > registerBudget {
		t.Errorf("Register() took %v, want it back within %v having opened nothing", took, registerBudget)
	}
}

func TestStopWithoutStartReleasesThePlugin(t *testing.T) {
	t.Parallel()

	p, err := fields.Register(sdk.Deps{DatabaseURL: silentDatabaseURL})
	if err != nil {
		t.Fatalf("Register() error = %v, want nil", err)
	}

	if err := p.Stop(t.Context()); err != nil {
		t.Errorf("Stop() without Start error = %v, want nil", err)
	}
}
