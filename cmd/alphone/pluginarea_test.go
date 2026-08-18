// SPDX-License-Identifier: Elastic-2.0

package main

import (
	"slices"
	"testing"

	"github.com/gopherium/alphone/internal/graphres"
	"github.com/gopherium/alphone/sdk"
)

func TestEveryPluginRouteAreaCanBeMintedIntoAToken(t *testing.T) {
	t.Parallel()

	registered, err := registerPlugins(sdk.Deps{Getenv: testGetenv(nil)})
	if err != nil {
		t.Fatalf("registerPlugins() error = %v, want nil", err)
	}

	areas := pluginAreas(registered)

	if len(areas) == 0 {
		t.Fatal("no plugin declares a route area, the collector reads nothing")
	}
	mintable := graphres.DeclaredAreas()
	for id, area := range areas {
		if !slices.Contains(mintable, area) {
			t.Errorf("%s holds its routes to %q, which no root field declares, so no token can reach them",
				id, area)
		}
	}
}
