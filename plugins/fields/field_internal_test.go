// SPDX-License-Identifier: Elastic-2.0

package fields

import (
	"errors"
	"slices"
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

// historyInput is the sub fields a history repeater is defined with.
var historyInput = []SubField{
	{Name: "date", Label: " Date ", Kind: kindDate},
	{Name: "comment", Label: "Comment", Kind: kindLongText},
}

func TestNewDefinitionHoldsARepeatersSubFieldsTrimmed(t *testing.T) {
	t.Parallel()

	definition, err := newDefinition("history", "History", "REPEATER", nil, historyInput...)

	if err != nil {
		t.Fatalf("newDefinition() error = %v, want nil", err)
	}
	want := []SubField{
		{Name: "date", Label: "Date", Kind: kindDate},
		{Name: "comment", Label: "Comment", Kind: kindLongText},
	}
	if definition.Kind != kindRepeater || !slices.Equal(definition.SubFields, want) {
		t.Errorf("definition = %+v, want a repeater holding %+v", definition, want)
	}
}

func TestNewDefinitionRefusesSubFieldsItCannotHold(t *testing.T) {
	t.Parallel()

	cases := map[string]struct {
		declared  string
		subFields []SubField
		want      error
	}{
		"a repeater without any":  {"REPEATER", nil, errSubFieldsRequired},
		"a plain field with some": {"DATE", historyInput, errSubFieldsUnexpected},
		"a repeater inside": {"REPEATER",
			[]SubField{{Name: "entries", Label: "Entries", Kind: kindRepeater}}, errSubFieldNested},
		"a malformed name": {"REPEATER",
			[]SubField{{Name: "Due Date", Label: "Due", Kind: kindDate}}, errSubFieldNameInvalid},
		"a name held twice": {"REPEATER", []SubField{
			{Name: "note", Label: "Note", Kind: kindText}, {Name: "note", Label: "Other", Kind: kindText},
		}, errSubFieldNameTaken},
		"an unknown kind": {"REPEATER",
			[]SubField{{Name: "due", Label: "Due", Kind: "TIMESTAMP"}}, errUnknownKind},
		"a blank label": {"REPEATER",
			[]SubField{{Name: "due", Label: "   ", Kind: kindDate}}, errBlankLabel},
		"a label beyond the cap": {"REPEATER",
			[]SubField{{Name: "due", Label: strings.Repeat("x", labelMax+1), Kind: kindDate}}, errLabelTooLong},
	}
	for name, testCase := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			_, err := newDefinition("history", "History", testCase.declared, nil, testCase.subFields...)

			if !errors.Is(err, testCase.want) {
				t.Errorf("error = %v, want %v", err, testCase.want)
			}
		})
	}
}

func TestEveryKindMapsToAScalar(t *testing.T) {
	t.Parallel()

	want := map[kind]string{
		kindText: "String", kindLongText: "String", kindNumber: "Int",
		kindBoolean: "Boolean", kindDate: "Date", kindSelect: "String",
		kindRepeater: "JSON",
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
