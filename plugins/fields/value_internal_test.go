// SPDX-License-Identifier: Elastic-2.0

package fields

import (
	"errors"
	"math"
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

func TestCheckValuesReportsEveryBadKeyAtOnce(t *testing.T) {
	t.Parallel()

	live := map[string]kind{"birthDate": kindDate, "loyaltyPoints": kindNumber}

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

	live := map[string]kind{"birthDate": kindDate, "loyaltyPoints": kindNumber}

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

	live := map[string]kind{"birthDate": kindDate}

	_, err := checkValues(live, map[string]any{"birthDate": "not a date"})

	if err == nil {
		t.Fatal("checkValues() error = nil, want the wrong kind refused")
	}
	if !strings.Contains(err.Error(), "birthDate") || !strings.Contains(err.Error(), "DATE") {
		t.Errorf("error = %v, want it to name the key and its kind", err)
	}
}
