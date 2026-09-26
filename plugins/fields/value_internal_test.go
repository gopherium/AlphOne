// SPDX-License-Identifier: Elastic-2.0

package fields

import (
	"encoding/json"
	"errors"
	"math"
	"reflect"
	"strings"
	"testing"
)

func TestCoerceAcceptsAValueOfItsKind(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		kind  kind
		given any
		want  any
	}{
		"text":            {kindText, "Maria Perez", "Maria Perez"},
		"long text":       {kindLongText, "a longer note", "a longer note"},
		"select":          {kindSelect, "home", "home"},
		"number":          {kindNumber, float64(42), int64(42)},
		"negative number": {kindNumber, float64(-7), int64(-7)},
		"boolean":         {kindBoolean, true, true},
		"date":            {kindDate, "1990-04-17", "1990-04-17"},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			got, err := coerce(tc.kind, tc.given)

			if err != nil {
				t.Fatalf("coerce() error = %v, want nil", err)
			}
			if got != tc.want {
				t.Errorf("coerce() = %#v, want %#v", got, tc.want)
			}
		})
	}
}

func TestCoerceRefusesAValueOfAnotherKind(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		kind  kind
		given any
	}{
		"text given a number":       {kindText, float64(42)},
		"number given text":         {kindNumber, "42"},
		"number given a fraction":   {kindNumber, 4.5},
		"boolean given text":        {kindBoolean, "true"},
		"date given plain text":     {kindDate, "not a date"},
		"date given a wrong format": {kindDate, "17-04-1990"},
		"date given a real time":    {kindDate, "1990-04-17T10:00:00Z"},
		"select given a boolean":    {kindSelect, false},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			_, err := coerce(tc.kind, tc.given)

			if !errors.Is(err, errWrongKind) {
				t.Errorf("coerce(%s, %#v) error = %v, want errWrongKind", tc.kind, tc.given, err)
			}
		})
	}
}

func TestCoerceRefusesANumberTheScalarCannotHold(t *testing.T) {
	t.Parallel()

	tests := map[string]float64{
		"above the Int ceiling": 2147483648,
		"below the Int floor":   -2147483649,
		"far above":             1e100,
		"positive infinity":     math.Inf(1),
		"negative infinity":     math.Inf(-1),
	}

	for name, given := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			_, err := coerce(kindNumber, given)

			if !errors.Is(err, errWrongKind) {
				t.Errorf("coerce(NUMBER, %v) error = %v, want errWrongKind", given, err)
			}
		})
	}
}

func TestCoerceReadsTheNumberTheDecoderProduces(t *testing.T) {
	t.Parallel()

	accepted, err := coerce(kindNumber, json.Number("420"))

	if err != nil {
		t.Fatalf("coerce() error = %v, want a decoded JSON number accepted", err)
	}
	if accepted != int64(420) {
		t.Errorf("coerce() = %#v, want int64(420)", accepted)
	}
	refused := map[string]json.Number{
		"a fraction":       json.Number("4.5"),
		"past the ceiling": json.Number("2147483648"),
		"not a number":     json.Number("nonsense"),
	}
	for name, given := range refused {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			if _, err := coerce(kindNumber, given); !errors.Is(err, errWrongKind) {
				t.Errorf("coerce(NUMBER, %q) error = %v, want errWrongKind", given, err)
			}
		})
	}
}

func TestCoerceAcceptsTheIntLimits(t *testing.T) {
	t.Parallel()

	for name, given := range map[string]float64{"ceiling": 2147483647, "floor": -2147483648} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			got, err := coerce(kindNumber, given)

			if err != nil {
				t.Fatalf("coerce(NUMBER, %v) error = %v, want nil", given, err)
			}
			if got != int64(given) {
				t.Errorf("coerce() = %#v, want %d", got, int64(given))
			}
		})
	}
}

func TestCoerceRefusesAnImpossibleDate(t *testing.T) {
	t.Parallel()

	_, err := coerce(kindDate, "1990-02-31")

	if !errors.Is(err, errWrongKind) {
		t.Errorf("error = %v, want errWrongKind", err)
	}
}

func TestCoerceKeepsNullAsAClear(t *testing.T) {
	t.Parallel()

	got, err := coerce(kindDate, nil)

	if err != nil {
		t.Fatalf("coerce() error = %v, want nil", err)
	}
	if got != nil {
		t.Errorf("coerce() = %#v, want nil so a write clears the value", got)
	}
}

// viewOver builds a catalogue view of the given kinds and repeater columns.
func viewOver(kinds map[string]kind, columns map[string][]SubField) *view {
	return &view{kinds: kinds, columns: columns}
}

// historyView holds a history repeater of a date, a comment and a count beside a birth date.
func historyView() *view {
	return viewOver(
		map[string]kind{"history": kindRepeater, "birthDate": kindDate},
		map[string][]SubField{"history": {
			{Name: "date", Label: "Date", Kind: kindDate},
			{Name: "comment", Label: "Comment", Kind: kindLongText},
			{Name: "count", Label: "Count", Kind: kindNumber},
		}},
	)
}

func TestCheckValuesReportsEveryBadKeyAtOnce(t *testing.T) {
	t.Parallel()

	live := viewOver(map[string]kind{"birthDate": kindDate, "loyaltyPoints": kindNumber}, nil)

	_, err := checkValues(live, map[string]any{
		"birthDate":     "1990-04-17",
		"neverDefined":  "x",
		"alsoUndefined": "y",
	})

	if err == nil {
		t.Fatal("checkValues() error = nil, want the undefined keys refused")
	}
	for _, want := range []string{"neverDefined", "alsoUndefined"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error = %v, want it to name %q", err, want)
		}
	}
	if strings.Contains(err.Error(), "birthDate") {
		t.Errorf("error = %v, want the good key left out", err)
	}
}

func TestCheckValuesCoercesEveryKnownKey(t *testing.T) {
	t.Parallel()

	live := viewOver(map[string]kind{"birthDate": kindDate, "loyaltyPoints": kindNumber}, nil)

	checked, err := checkValues(live, map[string]any{
		"birthDate":     "1990-04-17",
		"loyaltyPoints": float64(420),
	})

	if err != nil {
		t.Fatalf("checkValues() error = %v, want nil", err)
	}
	if checked["birthDate"] != "1990-04-17" || checked["loyaltyPoints"] != int64(420) {
		t.Errorf("checked = %#v, want both values coerced", checked)
	}
}

func TestCheckValuesNamesTheKindItRefused(t *testing.T) {
	t.Parallel()

	live := viewOver(map[string]kind{"birthDate": kindDate}, nil)

	_, err := checkValues(live, map[string]any{"birthDate": "not a date"})

	if err == nil {
		t.Fatal("checkValues() error = nil, want the wrong kind refused")
	}
	if !strings.Contains(err.Error(), "birthDate") || !strings.Contains(err.Error(), "DATE") {
		t.Errorf("error = %v, want it to name the key and its kind", err)
	}
}

func TestCheckValuesKeepsRepeaterRowsInOrder(t *testing.T) {
	t.Parallel()

	checked, err := checkValues(historyView(), map[string]any{"history": []any{
		map[string]any{"date": "2026-09-10", "comment": "Sent the offer", "count": float64(2)},
		map[string]any{"date": "2026-09-01", "comment": "First call"},
	}})

	if err != nil {
		t.Fatalf("checkValues() error = %v, want nil", err)
	}
	want := []map[string]any{
		{"date": "2026-09-10", "comment": "Sent the offer", "count": int64(2)},
		{"date": "2026-09-01", "comment": "First call"},
	}
	if !reflect.DeepEqual(checked["history"], want) {
		t.Errorf("history = %#v, want the rows in the order written with every cell coerced", checked["history"])
	}
}

func TestCheckValuesReportsEveryBadRowKeyAtOnce(t *testing.T) {
	t.Parallel()

	_, err := checkValues(historyView(), map[string]any{
		"history": []any{
			map[string]any{"date": "2026-09-01", "mood": "happy"},
			map[string]any{"size": "large"},
		},
		"neverDefined": "x",
	})

	if !errors.Is(err, errNoField) {
		t.Fatalf("checkValues() error = %v, want errNoField", err)
	}
	if !strings.HasSuffix(err.Error(), "history[0].mood, history[1].size, neverDefined") {
		t.Errorf("error = %v, want every bad key named by its path, sorted", err)
	}
}

func TestCheckValuesNamesTheCellItRefused(t *testing.T) {
	t.Parallel()

	_, err := checkValues(historyView(), map[string]any{"history": []any{
		map[string]any{"date": "2026-09-01"},
		map[string]any{"date": "not a date"},
	}})

	if !errors.Is(err, errWrongKind) || !strings.Contains(err.Error(), "history[1].date expects DATE") {
		t.Errorf("checkValues() error = %v, want the cell named with its kind", err)
	}
}

func TestCheckValuesDropsNullCellsAndEmptyRows(t *testing.T) {
	t.Parallel()

	checked, err := checkValues(historyView(), map[string]any{"history": []any{
		map[string]any{"date": "2026-09-01", "comment": nil},
		map[string]any{},
		map[string]any{"comment": nil},
	}})

	if err != nil {
		t.Fatalf("checkValues() error = %v, want nil", err)
	}
	want := []map[string]any{{"date": "2026-09-01"}}
	if !reflect.DeepEqual(checked["history"], want) {
		t.Errorf("history = %#v, want only the filled cell kept", checked["history"])
	}
}

func TestCheckValuesClearsARepeaterWrittenWithoutRows(t *testing.T) {
	t.Parallel()

	for name, given := range map[string]any{
		"no rows":         []any{},
		"null":            nil,
		"only empty rows": []any{map[string]any{}, map[string]any{"date": nil}},
	} {
		checked, err := checkValues(historyView(), map[string]any{"history": given})

		if err != nil {
			t.Fatalf("%s: checkValues() error = %v, want nil", name, err)
		}
		if held, present := checked["history"]; !present || held != nil {
			t.Errorf("%s: history = %#v, want null so the write clears it", name, held)
		}
	}
}

func TestCheckValuesRefusesRowsOfAnotherShape(t *testing.T) {
	t.Parallel()

	cases := map[string]struct {
		given any
		want  string
	}{
		"a value that is not a list":  {"a text", "history expects REPEATER"},
		"a row that is not an object": {[]any{"a text"}, "history[0] expects a row"},
	}
	for name, testCase := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			_, err := checkValues(historyView(), map[string]any{"history": testCase.given})

			if !errors.Is(err, errWrongKind) || !strings.Contains(err.Error(), testCase.want) {
				t.Errorf("checkValues() error = %v, want %q", err, testCase.want)
			}
		})
	}
}
