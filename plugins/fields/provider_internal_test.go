// SPDX-License-Identifier: Elastic-2.0

package fields

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/google/uuid"

	"github.com/gopherium/alphone/sdk"
)

var _ sdk.FieldProvider = (*Plugin)(nil)

// labelled builds a live definition carrying a chosen label.
func labelled(t *testing.T, name, label, declared string) Definition {
	t.Helper()
	definition, err := newDefinition(name, label, declared, nil)
	if err != nil {
		t.Fatalf("newDefinition(%q) error = %v, want nil", name, err)
	}
	return definition
}

// define stores a definition and reloads the catalogue behind it.
func define(t *testing.T, p *Plugin, definition Definition) {
	t.Helper()
	if err := p.store.define(t.Context(), definition); err != nil {
		t.Fatalf("define(%q) error = %v, want nil", definition.Name, err)
	}
	if err := p.catalog.reload(t.Context()); err != nil {
		t.Fatalf("reload() error = %v, want nil", err)
	}
}

func TestTypedTextReadsTheValueItsKindHolds(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		kind kind
		text string
		want any
	}{
		"text":            {kindText, "Maria Perez", "Maria Perez"},
		"long text":       {kindLongText, "a longer note", "a longer note"},
		"select":          {kindSelect, "home", "home"},
		"date":            {kindDate, "1990-04-17", "1990-04-17"},
		"number":          {kindNumber, "420", json.Number("420")},
		"negative number": {kindNumber, "-7", json.Number("-7")},
		"signed number":   {kindNumber, "+7", json.Number("+7")},
		"boolean true":    {kindBoolean, "true", true},
		"boolean yes":     {kindBoolean, "yes", true},
		"boolean one":     {kindBoolean, "1", true},
		"boolean false":   {kindBoolean, "false", false},
		"boolean no":      {kindBoolean, "no", false},
		"boolean zero":    {kindBoolean, "0", false},
		"boolean upper":   {kindBoolean, "YES", true},
		"boolean mixed":   {kindBoolean, "No", false},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			if got := typedText(tc.kind, tc.text); got != tc.want {
				t.Errorf("typedText(%s, %q) = %#v, want %#v", tc.kind, tc.text, got, tc.want)
			}
		})
	}
}

func TestTypedTextLeavesUnreadableTextAsWritten(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		kind kind
		text string
	}{
		"number given words":              {kindNumber, "four hundred"},
		"number given a fraction":         {kindNumber, "4.5"},
		"number given scientific writing": {kindNumber, "1e5"},
		"number given a stray comma":      {kindNumber, "1,000"},
		"boolean given words":             {kindBoolean, "maybe"},
		"boolean given a number":          {kindBoolean, "2"},
		"an unknown kind":                 {kind("TIMESTAMP"), "1990-04-17"},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			if got := typedText(tc.kind, tc.text); got != tc.text {
				t.Errorf("typedText(%s, %q) = %#v, want the text left for coerce to refuse", tc.kind, tc.text, got)
			}
		})
	}
}

func TestLiveContactFieldsListsEveryNameWithItsLabel(t *testing.T) {
	t.Parallel()

	p := newMigratedPlugin(t)
	define(t, p, labelled(t, "birthDate", "Birth date", "DATE"))
	define(t, p, labelled(t, "loyaltyPoints", "Loyalty points", "NUMBER"))

	listed, err := p.LiveContactFields(t.Context())

	if err != nil {
		t.Fatalf("LiveContactFields() error = %v, want nil", err)
	}
	want := map[string]string{"birthDate": "Birth date", "loyaltyPoints": "Loyalty points"}
	if len(listed) != len(want) {
		t.Fatalf("listed = %d fields, want %d", len(listed), len(want))
	}
	for _, field := range listed {
		if want[field.Name] != field.Label {
			t.Errorf("%s is labelled %q, want %q", field.Name, field.Label, want[field.Name])
		}
	}
}

func TestLiveContactFieldsLeavesOutAnArchivedField(t *testing.T) {
	t.Parallel()

	p := newMigratedPlugin(t)
	definition := labelled(t, "birthDate", "Birth date", "DATE")
	define(t, p, definition)
	if err := p.store.archive(t.Context(), definition.ID); err != nil {
		t.Fatalf("archive() error = %v, want nil", err)
	}

	listed, err := p.LiveContactFields(t.Context())

	if err != nil {
		t.Fatalf("LiveContactFields() error = %v, want nil", err)
	}
	if len(listed) != 0 {
		t.Errorf("listed = %#v, want the archived field left out", listed)
	}
}

func TestLiveContactFieldsReportsAReadFailure(t *testing.T) {
	t.Parallel()

	p := newMigratedPlugin(t)
	if err := p.Stop(t.Context()); err != nil {
		t.Fatalf("Stop() error = %v, want nil", err)
	}

	if _, err := p.LiveContactFields(t.Context()); err == nil {
		t.Error("LiveContactFields() error = nil, want the unreadable catalogue reported")
	}
}

func TestWriteContactFieldTextsStoresTheValueEachKindHolds(t *testing.T) {
	t.Parallel()

	p := newMigratedPlugin(t)
	define(t, p, labelled(t, "birthDate", "Birth date", "DATE"))
	define(t, p, labelled(t, "loyaltyPoints", "Loyalty points", "NUMBER"))
	define(t, p, labelled(t, "subscribed", "Subscribed", "BOOLEAN"))
	maria := seedContact(t, p, "Maria Perez")

	err := p.WriteContactFieldTexts(t.Context(), maria, map[string]string{
		"birthDate": "1990-04-17", "loyaltyPoints": "420", "subscribed": "yes",
	})

	if err != nil {
		t.Fatalf("WriteContactFieldTexts() error = %v, want nil", err)
	}
	held, err := p.store.valuesFor(t.Context(), []uuid.UUID{maria})
	if err != nil {
		t.Fatalf("valuesFor() error = %v, want nil", err)
	}
	values := held[maria]
	if values["birthDate"] != "1990-04-17" {
		t.Errorf("birthDate = %#v, want the written date", values["birthDate"])
	}
	if values["loyaltyPoints"] != float64(420) {
		t.Errorf("loyaltyPoints = %#v, want the number stored", values["loyaltyPoints"])
	}
	if values["subscribed"] != true {
		t.Errorf("subscribed = %#v, want true", values["subscribed"])
	}
}

func TestWriteContactFieldTextsTrimsBeforeReading(t *testing.T) {
	t.Parallel()

	p := newMigratedPlugin(t)
	define(t, p, labelled(t, "loyaltyPoints", "Loyalty points", "NUMBER"))
	maria := seedContact(t, p, "Maria Perez")

	err := p.WriteContactFieldTexts(t.Context(), maria, map[string]string{"loyaltyPoints": "  420  "})

	if err != nil {
		t.Fatalf("WriteContactFieldTexts() error = %v, want nil", err)
	}
	held, err := p.store.valuesFor(t.Context(), []uuid.UUID{maria})
	if err != nil {
		t.Fatalf("valuesFor() error = %v, want nil", err)
	}
	if held[maria]["loyaltyPoints"] != float64(420) {
		t.Errorf("loyaltyPoints = %#v, want the trimmed number", held[maria]["loyaltyPoints"])
	}
}

func TestWriteContactFieldTextsLeavesAnEmptyTextUnwritten(t *testing.T) {
	t.Parallel()

	p := newMigratedPlugin(t)
	define(t, p, labelled(t, "birthDate", "Birth date", "DATE"))
	maria := seedContact(t, p, "Maria Perez")

	err := p.WriteContactFieldTexts(t.Context(), maria, map[string]string{"birthDate": "   "})

	if err != nil {
		t.Fatalf("WriteContactFieldTexts() error = %v, want nil", err)
	}
	held, err := p.store.valuesFor(t.Context(), []uuid.UUID{maria})
	if err != nil {
		t.Fatalf("valuesFor() error = %v, want nil", err)
	}
	if _, written := held[maria]["birthDate"]; written {
		t.Errorf("values = %#v, want an empty text to store nothing", held[maria])
	}
}

func TestWriteContactFieldTextsRefusesATextOfAnotherKind(t *testing.T) {
	t.Parallel()

	p := newMigratedPlugin(t)
	define(t, p, labelled(t, "birthDate", "Birth date", "DATE"))
	maria := seedContact(t, p, "Maria Perez")

	err := p.WriteContactFieldTexts(t.Context(), maria, map[string]string{"birthDate": "not a date"})

	if !errors.Is(err, sdk.ErrInvalidFieldText) {
		t.Fatalf("error = %v, want sdk.ErrInvalidFieldText", err)
	}
	for _, want := range []string{"birthDate", "DATE"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error = %v, want it to name %q", err, want)
		}
	}
}

func TestWriteContactFieldTextsRefusesANumberTheScalarCannotHold(t *testing.T) {
	t.Parallel()

	p := newMigratedPlugin(t)
	define(t, p, labelled(t, "loyaltyPoints", "Loyalty points", "NUMBER"))
	maria := seedContact(t, p, "Maria Perez")

	err := p.WriteContactFieldTexts(t.Context(), maria, map[string]string{"loyaltyPoints": "2147483648"})

	if !errors.Is(err, sdk.ErrInvalidFieldText) {
		t.Errorf("error = %v, want the Int ceiling enforced through coerce", err)
	}
}

func TestWriteContactFieldTextsRefusesANameNoFieldHolds(t *testing.T) {
	t.Parallel()

	p := newMigratedPlugin(t)
	maria := seedContact(t, p, "Maria Perez")

	err := p.WriteContactFieldTexts(t.Context(), maria, map[string]string{"neverDefined": "x"})

	if !errors.Is(err, sdk.ErrInvalidFieldText) {
		t.Fatalf("error = %v, want sdk.ErrInvalidFieldText", err)
	}
	if !strings.Contains(err.Error(), "neverDefined") {
		t.Errorf("error = %v, want it to name the undefined field", err)
	}
}

func TestWriteContactFieldTextsWritesNothingWhenEveryTextIsEmpty(t *testing.T) {
	t.Parallel()

	p := newMigratedPlugin(t)
	if err := p.Stop(t.Context()); err != nil {
		t.Fatalf("Stop() error = %v, want nil", err)
	}

	err := p.WriteContactFieldTexts(t.Context(), uuid.Must(uuid.NewV7()), map[string]string{"birthDate": ""})

	if err != nil {
		t.Errorf("error = %v, want no store call for an empty text", err)
	}
}

func TestCheckContactFieldTextsRefusesWithoutWriting(t *testing.T) {
	t.Parallel()

	p := newMigratedPlugin(t)
	define(t, p, labelled(t, "birthDate", "Birth date", "DATE"))
	maria := seedContact(t, p, "Maria Perez")

	err := p.CheckContactFieldTexts(t.Context(), map[string]string{"birthDate": "not a date"})

	if !errors.Is(err, sdk.ErrInvalidFieldText) {
		t.Fatalf("error = %v, want sdk.ErrInvalidFieldText", err)
	}
	held, err := p.store.valuesFor(t.Context(), []uuid.UUID{maria})
	if err != nil {
		t.Fatalf("valuesFor() error = %v, want nil", err)
	}
	if len(held) != 0 {
		t.Errorf("values = %#v, want the check to store nothing", held)
	}
}

func TestCheckContactFieldTextsAcceptsTextEveryKindHolds(t *testing.T) {
	t.Parallel()

	p := newMigratedPlugin(t)
	define(t, p, labelled(t, "birthDate", "Birth date", "DATE"))
	define(t, p, labelled(t, "loyaltyPoints", "Loyalty points", "NUMBER"))

	err := p.CheckContactFieldTexts(t.Context(), map[string]string{
		"birthDate": "1990-04-17", "loyaltyPoints": "420",
	})

	if err != nil {
		t.Errorf("CheckContactFieldTexts() error = %v, want nil", err)
	}
}
