// SPDX-License-Identifier: Elastic-2.0

package graphres_test

import (
	"errors"
	"slices"
	"testing"

	"github.com/gopherium/alphone/internal/apitoken"
	"github.com/gopherium/alphone/internal/graphres"
)

func TestDeclaredAreasNamesEveryAreaTheBuiltSchemaCarries(t *testing.T) {
	t.Parallel()

	areas := graphres.DeclaredAreas()

	for _, want := range []string{"auth", "contacts", "meta", "tasks", "tokens", "users", "webhooks"} {
		if !slices.Contains(areas, want) {
			t.Errorf("areas = %v, want the core area %q among them", areas, want)
		}
	}
	for _, want := range []string{"fields", "imports", "whatsapp"} {
		if !slices.Contains(areas, want) {
			t.Errorf("areas = %v, want the plugin area %q, the set reads the built schema", areas, want)
		}
	}
	if slices.Contains(areas, "") {
		t.Error("areas hold the empty string, want every entry to name an area")
	}
}

func TestValidateScopesRefusesAnAreaNoSchemaDeclares(t *testing.T) {
	t.Parallel()

	err := graphres.ValidateScopes(apitoken.ParseScopes("contact:read"))

	if !errors.Is(err, apitoken.ErrUnknownArea) {
		t.Errorf("ValidateScopes() error = %v, want %v", err, apitoken.ErrUnknownArea)
	}
}

func TestValidateScopesKeepsTheWildcardValid(t *testing.T) {
	t.Parallel()

	if err := graphres.ValidateScopes(apitoken.Full()); err != nil {
		t.Errorf("ValidateScopes(*) error = %v, want nil, every grandfathered token holds it", err)
	}
}

func TestValidateScopesStillRefusesAMalformedScope(t *testing.T) {
	t.Parallel()

	err := graphres.ValidateScopes(apitoken.ParseScopes("tasks:admin"))

	if !errors.Is(err, apitoken.ErrMalformedScope) {
		t.Errorf("ValidateScopes() error = %v, want %v", err, apitoken.ErrMalformedScope)
	}
}

func TestValidateScopesRefusesAGrantCarryingNothing(t *testing.T) {
	t.Parallel()

	if err := graphres.ValidateScopes(nil); !errors.Is(err, apitoken.ErrNoScopes) {
		t.Errorf("ValidateScopes(nil) error = %v, want %v", err, apitoken.ErrNoScopes)
	}
}
