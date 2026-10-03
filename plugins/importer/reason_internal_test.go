// SPDX-License-Identifier: Elastic-2.0

package importer

import (
	"encoding/csv"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/gopherium/alphone/sdk"
)

func TestMalformedReasonNamesTheLineOfAMisplacedQuote(t *testing.T) {
	t.Parallel()

	for name, fault := range map[string]error{"bare quote": csv.ErrBareQuote, "quoted field": csv.ErrQuote} {
		err := &csv.ParseError{StartLine: 2, Line: 2, Column: 6, Err: fault}

		got := malformedReason(err, 0)

		want := rowReason{Code: reasonQuoteMisplaced, Meta: map[string]any{"line": 2}}
		if !reflect.DeepEqual(got, want) {
			t.Errorf("%s: malformedReason() = %+v, want %+v", name, got, want)
		}
	}
}

func TestMalformedReasonCountsTheLinesAheadOfTheReader(t *testing.T) {
	t.Parallel()

	err := &csv.ParseError{StartLine: 2, Line: 2, Column: 6, Err: csv.ErrBareQuote}

	got := malformedReason(err, 1)

	want := rowReason{Code: reasonQuoteMisplaced, Meta: map[string]any{"line": 3}}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("malformedReason() = %+v, want %+v", got, want)
	}
}

func TestMalformedReasonLeavesOtherReaderErrorsUnnamed(t *testing.T) {
	t.Parallel()

	tests := map[string]error{
		"a field count error": &csv.ParseError{StartLine: 3, Line: 3, Err: csv.ErrFieldCount},
		"a workbook error":    errors.New("the cell reference is out of range"),
	}

	for name, err := range tests {
		got := malformedReason(err, 0)

		want := rowReason{Code: reasonMalformed, Meta: map[string]any{}}
		if !reflect.DeepEqual(got, want) {
			t.Errorf("%s: malformedReason() = %+v, want %+v", name, got, want)
		}
	}
}

func TestFieldTextReasonNamesTheFieldAndKind(t *testing.T) {
	t.Parallel()

	err := fmt.Errorf("checking the row: %w", sdk.FieldTextError{
		Reason: sdk.FieldTextKindMismatch, Fields: []string{"birthDate"}, Kind: "DATE",
		Err: errors.New("birthDate expects DATE"),
	})

	got := fieldTextReason(err)

	want := rowReason{Code: sdk.FieldTextKindMismatch, Meta: map[string]any{"field": "birthDate", "kind": "DATE"}}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("fieldTextReason() = %+v, want %+v", got, want)
	}
}

func TestFieldTextReasonNamesUnknownFields(t *testing.T) {
	t.Parallel()

	err := sdk.FieldTextError{
		Reason: sdk.FieldTextFieldUnknown, Fields: []string{"birthDate", "shoeSize"},
		Err: errors.New("no live field holds birthDate, shoeSize"),
	}

	got := fieldTextReason(err)

	want := rowReason{Code: sdk.FieldTextFieldUnknown, Meta: map[string]any{"fields": []string{"birthDate", "shoeSize"}}}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("fieldTextReason() = %+v, want %+v", got, want)
	}
}

func TestFieldTextReasonFallsBackForAnErrorItCannotRead(t *testing.T) {
	t.Parallel()

	provider := errors.New("the provider will not store it")
	tests := map[string]error{
		"a plain sentinel": fmt.Errorf("%w: birthDate", sdk.ErrInvalidFieldText),
		"an unknown reason": sdk.FieldTextError{
			Reason: "field_repeater_entries_only", Fields: []string{"history"}, Err: provider,
		},
		"a kind mismatch without a kind": sdk.FieldTextError{
			Reason: sdk.FieldTextKindMismatch, Fields: []string{"birthDate"}, Err: provider,
		},
		"a kind mismatch naming two fields": sdk.FieldTextError{
			Reason: sdk.FieldTextKindMismatch, Fields: []string{"anniversary", "birthDate"}, Kind: "DATE", Err: provider,
		},
		"an unknown field error naming no field": sdk.FieldTextError{
			Reason: sdk.FieldTextFieldUnknown, Err: provider,
		},
	}

	for name, err := range tests {
		got := fieldTextReason(err)

		want := rowReason{Code: reasonFieldTextRefused, Meta: map[string]any{}}
		if !reflect.DeepEqual(got, want) {
			t.Errorf("%s: fieldTextReason() = %+v, want %+v", name, got, want)
		}
	}
}

func TestIdentityTakenReasonNamesTheOwner(t *testing.T) {
	t.Parallel()

	got := identityTakenReason("Maria Perez")

	want := rowReason{Code: reasonIdentityTakenBy, Meta: map[string]any{"ownerName": "Maria Perez"}}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("identityTakenReason() = %+v, want %+v", got, want)
	}
}

func TestNewReasonNeverCarriesANilMeta(t *testing.T) {
	t.Parallel()

	got := newReason(reasonIncomplete, nil)

	if got.Meta == nil || len(got.Meta) != 0 {
		t.Errorf("Meta = %#v, want an empty map so the stored object always carries one", got.Meta)
	}
}

func TestOptionalReasonStoresNothingForAnEmptyReason(t *testing.T) {
	t.Parallel()

	if got := optionalReason(rowReason{}); got != nil {
		t.Errorf("optionalReason(empty) = %+v, want nil", got)
	}
	incomplete := newReason(reasonIncomplete, nil)
	if got := optionalReason(incomplete); got == nil || got.Code != reasonIncomplete {
		t.Errorf("optionalReason(incomplete) = %+v, want the reason kept", got)
	}
}

func TestUnreadableReportsOnlyTheNoteOfARowTheReaderCouldNotRead(t *testing.T) {
	t.Parallel()

	quote := newReason(reasonQuoteMisplaced, map[string]any{"line": 2})
	malformed := newReason(reasonMalformed, nil)
	short := cellCountReason(1, 3)
	tests := map[string]struct {
		reason *rowReason
		want   bool
	}{
		"a misplaced quote": {&quote, true},
		"a malformed row":   {&malformed, true},
		"a short row":       {&short, false},
		"no note":           {nil, false},
	}

	for name, tc := range tests {
		if got := tc.reason.unreadable(); got != tc.want {
			t.Errorf("%s: unreadable() = %t, want %t", name, got, tc.want)
		}
	}
}

func TestEveryRowReasonHasAFrontendMessage(t *testing.T) {
	t.Parallel()

	templates, err := os.ReadFile(filepath.Join("frontend", "rowReasons.ts"))
	if err != nil {
		t.Fatalf("reading the frontend row reasons: %v", err)
	}

	for _, code := range rowReasonCodes {
		if !strings.Contains(string(templates), "\t"+code+": __(") {
			t.Errorf("row reason %q has no frontend message, want the screens to speak it", code)
		}
	}
}

func TestCellCountReasonNamesTheCellsAndTheColumns(t *testing.T) {
	t.Parallel()

	got := cellCountReason(4, 3)

	want := rowReason{Code: reasonCellCountMismatch, Meta: map[string]any{"cells": 4, "columns": 3}}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("cellCountReason() = %+v, want %+v", got, want)
	}
}
