// SPDX-License-Identifier: Elastic-2.0

package apitoken_test

import (
	"errors"
	"testing"

	"github.com/gopherium/alphone/internal/apitoken"
)

func TestParseScopesReadsTheStoredList(t *testing.T) {
	t.Parallel()

	scopes := apitoken.ParseScopes("tasks:write contacts:read")

	if got, want := scopes.String(), "contacts:read tasks:write"; got != want {
		t.Errorf("String() = %q, want %q", got, want)
	}
}

func TestParseScopesCanonicalisesOrderAndRepeats(t *testing.T) {
	t.Parallel()

	scopes := apitoken.ParseScopes("  tasks:write   contacts:read  tasks:write ")

	if got, want := scopes.String(), "contacts:read tasks:write"; got != want {
		t.Errorf("String() = %q, want %q, want sorted without repeats", got, want)
	}
}

func TestParseScopesReadsAnEmptyListAsNoScopes(t *testing.T) {
	t.Parallel()

	scopes := apitoken.ParseScopes("   ")

	if len(scopes) != 0 {
		t.Errorf("ParseScopes(blank) = %v, want no scopes", scopes)
	}
	if got := scopes.String(); got != "" {
		t.Errorf("String() = %q, want empty", got)
	}
}

func TestFullIsTheWildcardALegacyTokenHolds(t *testing.T) {
	t.Parallel()

	if got, want := apitoken.Full().String(), "*"; got != want {
		t.Errorf("Full() = %q, want %q", got, want)
	}
}

func TestAllowsReadsWhenTheAreaIsGrantedForReading(t *testing.T) {
	t.Parallel()

	scopes := apitoken.ParseScopes("contacts:read")

	if !scopes.Allows("contacts", false) {
		t.Error("Allows(contacts, read) = false, want true")
	}
	if scopes.Allows("contacts", true) {
		t.Error("Allows(contacts, write) = true, want false, reading never grants writing")
	}
}

func TestAllowsReadsWhenTheAreaIsGrantedForWriting(t *testing.T) {
	t.Parallel()

	scopes := apitoken.ParseScopes("tasks:write")

	if !scopes.Allows("tasks", true) {
		t.Error("Allows(tasks, write) = false, want true")
	}
	if !scopes.Allows("tasks", false) {
		t.Error("Allows(tasks, read) = false, want true, writing implies reading")
	}
}

func TestAllowsRefusesAnAreaThatWasNeverGranted(t *testing.T) {
	t.Parallel()

	scopes := apitoken.ParseScopes("contacts:write")

	if scopes.Allows("tasks", false) {
		t.Error("Allows(tasks, read) = true, want false")
	}
}

func TestAllowsGrantsEveryAreaToTheWildcard(t *testing.T) {
	t.Parallel()

	scopes := apitoken.Full()

	if !scopes.Allows("anything", true) {
		t.Error("Allows(anything, write) = false, want true under the wildcard")
	}
}

func TestAllowsRefusesEverythingWhenNoScopeIsHeld(t *testing.T) {
	t.Parallel()

	var scopes apitoken.Scopes

	if scopes.Allows("contacts", false) {
		t.Error("Allows(contacts, read) = true, want false when no scope is held")
	}
}

func TestValidateAcceptsTheGrammar(t *testing.T) {
	t.Parallel()

	for _, stored := range []string{"*", "contacts:read", "contacts:write tasks:read"} {
		if err := apitoken.ParseScopes(stored).Validate(); err != nil {
			t.Errorf("Validate(%q) error = %v, want nil", stored, err)
		}
	}
}

func TestValidateRejectsMalformedEntries(t *testing.T) {
	t.Parallel()

	for _, stored := range []string{"contacts", "contacts:admin", ":read", "contacts:read:write", "**"} {
		if err := apitoken.ParseScopes(stored).Validate(); !errors.Is(err, apitoken.ErrMalformedScope) {
			t.Errorf("Validate(%q) error = %v, want %v", stored, err, apitoken.ErrMalformedScope)
		}
	}
}

func TestValidateRejectsAnEntryTheStoredFormWouldSplit(t *testing.T) {
	t.Parallel()

	for _, entry := range []string{"* x:read", "a b:read", "contacts :read", "contacts: read", "\tx:read"} {
		granted := apitoken.Scopes{entry}
		if err := granted.Validate(); !errors.Is(err, apitoken.ErrMalformedScope) {
			t.Errorf("Validate(%q) error = %v, want %v", entry, err, apitoken.ErrMalformedScope)
		}
	}
}

func TestEveryAcceptedScopeSurvivesTheStoredForm(t *testing.T) {
	t.Parallel()

	granted := apitoken.Scopes{"*", "contacts:read", "tasks:write"}
	if err := granted.Validate(); err != nil {
		t.Fatalf("Validate() error = %v, want nil", err)
	}

	reloaded := apitoken.ParseScopes(granted.String())

	if got, want := reloaded.String(), granted.String(); got != want {
		t.Errorf("reloaded = %q, want %q, the stored form must round trip", got, want)
	}
}

func TestValidateRejectsHoldingNoScopeAtAll(t *testing.T) {
	t.Parallel()

	if err := apitoken.ParseScopes("").Validate(); !errors.Is(err, apitoken.ErrNoScopes) {
		t.Errorf("Validate(empty) error = %v, want %v", err, apitoken.ErrNoScopes)
	}
}
