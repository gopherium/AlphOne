// SPDX-License-Identifier: Elastic-2.0

package message

import (
	"math/rand/v2"
	"slices"
	"strings"
	"testing"

	"github.com/microcosm-cc/bluemonday/css"
)

// checkedValues lists the style values every check is compared with bluemonday on.
var checkedValues = []string{
	"", ",", ",,", "red", "red,blue", "red,", "#fff", "#fff,#fff", "rgb(1,2,3)", "rgb(1,2,3),red", "10px",
	"10px,10px", "auto", "auto,auto", "auto, 10px", "initial, inherit", "center, left", "100,700", "'a', b", "a b",
	"\U000000A0red\U000000A0,\U000000A0blue\U000000A0", "blac\U0000212A", "\U00000130nherit",
}

// valuePieces lists the pieces the seeded style values are made of.
var valuePieces = []string{
	"red", "RED", "tan", "blue", "lightsteeelblue", "lightsteelblue", "transparent", "initial", "inherit", "auto",
	"left", "center", "bold", "100", "700", "normal", "italic", "collapse", "separate", "nowrap", "pre", "pre-wrap",
	"top", "sub", "middle", "medium", "small", "x-small", "10px", "-1.5em", "0", ".5", "5.", "10%", "#fff", "#ffff",
	"#ffffff", "#fffffff", "#ffffffff", "rgb(1,2,3)", "rgb( 1, 2, 3)", "rgba(1,2,3,0.5)", "hsl(1,2%,3%)",
	"hsla( 1,2%,3%,0)", "hsl(120,%,%)", "rgb(", ")", "(", ",", ",", ",", ", ", " ,", " ", "\t", "\n", "\xc2\xa0",
	"\xe2\x84\xaa", "\xc4\xb0", "\xff", "a", "x", "-", "%", "'a'", "arial", "sans-serif", "''", "\xe3\x80\x80",
	"\xc2\x85", "\xe2\x80\x8b",
}

// seededValue answers a lower cased style value of 1 to 12 pieces drawn by r.
func seededValue(r *rand.Rand) string {
	var value strings.Builder
	for range 1 + r.IntN(12) {
		value.WriteString(valuePieces[r.IntN(len(valuePieces))])
	}
	return strings.ToLower(value.String())
}

// comparedValues answers checkedValues followed by 20,000 seeded values.
func comparedValues() []string {
	values := slices.Clone(checkedValues)
	r := rand.New(rand.NewPCG(1, 2))
	for range 20000 {
		values = append(values, seededValue(r))
	}
	return values
}

// compareWithBluemonday fails t for each of values on which check answers otherwise than bluemonday checks property.
func compareWithBluemonday(t *testing.T, property string, check func(string) bool, values []string) {
	t.Helper()
	handler := css.GetDefaultHandler(property)
	for _, value := range values {
		if got, want := check(value), handler(value); got != want {
			t.Errorf("styleChecks[%q](%q) = %t, want %t as bluemonday answers", property, value, got, want)
		}
	}
}

func TestStyleChecksAnswerAsBluemonday(t *testing.T) {
	t.Parallel()

	if len(styleChecks) != len(allowedStyles) {
		t.Errorf("styleChecks holds %d checks, want one for each of the %d allowed styles", len(styleChecks),
			len(allowedStyles))
	}
	values := comparedValues()

	for _, property := range allowedStyles {
		check, ok := styleChecks[property]
		if !ok {
			t.Errorf("styleChecks holds no check for %s", property)
			continue
		}
		compareWithBluemonday(t, property, check, values)
	}
}
