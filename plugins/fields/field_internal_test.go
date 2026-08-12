// SPDX-License-Identifier: Elastic-2.0

package fields

import (
	"errors"
	"strings"
	"testing"
)

func TestNewDefinitionHoldsWhatItWasGiven(t *testing.T) {
	t.Parallel()

	definition, err := newDefinition("birthDate", "Birth date", "DATE", nil)

	if err != nil {
		t.Fatalf("newDefinition() error = %v, want nil", err)
	}
	if definition.ID.String() == "" {
		t.Error("id is blank, want a generated identifier")
	}
	if definition.Name != "birthDate" || definition.Label != "Birth date" {
		t.Errorf("definition = %+v, want the given name and label", definition)
	}
	if definition.Kind != kindDate {
		t.Errorf("kind = %q, want %q", definition.Kind, kindDate)
	}
	if definition.ArchivedAt != nil {
		t.Error("archivedAt is set, want a live definition")
	}
}

func TestNewDefinitionRefusesAMalformedName(t *testing.T) {
	t.Parallel()

	names := map[string]string{
		"a space":       "birth date",
		"a leading cap": "BirthDate",
		"a dash":        "birth-date",
		"an underscore": "birth_date",
		"a digit first": "1birthDate",
		"blank":         "",
		"two words":     "birth Date",
	}

	for label, name := range names {
		t.Run(label, func(t *testing.T) {
			t.Parallel()

			_, err := newDefinition(name, "Birth date", "DATE", nil)

			if !errors.Is(err, errMalformedName) {
				t.Errorf("newDefinition(%q) error = %v, want errMalformedName", name, err)
			}
		})
	}
}

func TestNewDefinitionAcceptsAWellFormedName(t *testing.T) {
	t.Parallel()

	for _, name := range []string{"a", "birthDate", "loyaltyPoints2", "x9"} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			if _, err := newDefinition(name, "Label", "TEXT", nil); err != nil {
				t.Errorf("newDefinition(%q) error = %v, want nil", name, err)
			}
		})
	}
}

func TestNewDefinitionRefusesAnUnknownKind(t *testing.T) {
	t.Parallel()

	_, err := newDefinition("birthDate", "Birth date", "TIMESTAMP", nil)

	if !errors.Is(err, errUnknownKind) {
		t.Errorf("error = %v, want errUnknownKind", err)
	}
}

func TestNewDefinitionRefusesABlankLabel(t *testing.T) {
	t.Parallel()

	_, err := newDefinition("birthDate", "   ", "DATE", nil)

	if !errors.Is(err, errBlankLabel) {
		t.Errorf("error = %v, want errBlankLabel", err)
	}
}

func TestNewDefinitionTrimsTheLabel(t *testing.T) {
	t.Parallel()

	definition, err := newDefinition("birthDate", "  Birth date  ", "DATE", nil)

	if err != nil {
		t.Fatalf("newDefinition() error = %v, want nil", err)
	}
	if definition.Label != "Birth date" {
		t.Errorf("label = %q, want it trimmed", definition.Label)
	}
}

func TestNewDefinitionRefusesAReservedName(t *testing.T) {
	t.Parallel()

	reserved := map[string]bool{"name": true, "tasks": true}

	_, err := newDefinition("name", "Name", "TEXT", reserved)

	if !errors.Is(err, errReservedName) {
		t.Errorf("error = %v, want errReservedName", err)
	}
}

func TestNewDefinitionRefusesALabelBeyondTheCap(t *testing.T) {
	t.Parallel()

	_, err := newDefinition("birthDate", strings.Repeat("x", labelMax+1), "DATE", nil)

	if !errors.Is(err, errLabelTooLong) {
		t.Errorf("error = %v, want errLabelTooLong", err)
	}
}

func TestNewDefinitionCountsLabelCharactersNotBytes(t *testing.T) {
	t.Parallel()

	accented := strings.Repeat("é", labelMax)

	if _, err := newDefinition("birthDate", accented, "DATE", nil); err != nil {
		t.Errorf("newDefinition() error = %v, want a %d character label accepted", err, labelMax)
	}
	if _, err := newDefinition("birthDate", accented+"é", "DATE", nil); !errors.Is(err, errLabelTooLong) {
		t.Errorf("error = %v, want errLabelTooLong one character past the cap", err)
	}
}

func TestEveryKindMapsToAScalar(t *testing.T) {
	t.Parallel()

	want := map[kind]string{
		kindText: "String", kindLongText: "String", kindNumber: "Int",
		kindBoolean: "Boolean", kindDate: "Date", kindSelect: "String",
	}

	for held, scalar := range want {
		if got := held.scalar(); got != scalar {
			t.Errorf("%q.scalar() = %q, want %q", held, got, scalar)
		}
	}
	if len(want) != len(kinds) {
		t.Errorf("kinds = %d, want every one of the %d mapped kinds", len(kinds), len(want))
	}
}
