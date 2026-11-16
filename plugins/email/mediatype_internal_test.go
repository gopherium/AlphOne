// SPDX-License-Identifier: Elastic-2.0

package email

import (
	"strings"
	"testing"
)

func TestStorableTypeKeepsTheMediaTypeAloneUpToItsLongestLength(t *testing.T) {
	t.Parallel()

	longestType := "application/" + strings.Repeat("x", mediaTypeMaxBytes-len("application/"))
	for name, c := range map[string]struct {
		contentType string
		want        string
	}{
		"a type with parameters":                {`application/pdf; name="quote.pdf"`, "application/pdf"},
		"a type in upper case":                  {"Image/PNG; Name=Photo.png", "image/png"},
		"a type of the longest length":          {longestType, longestType},
		"a type of the longest with parameters": {longestType + `; name="quote.pdf"`, longestType},
		"a type one byte over the longest":      {longestType + "x", octetStream},
		"a type with no subtype":                {"inline", octetStream},
		"a type that does not parse":            {"not a type", octetStream},
		"no type":                               {"", octetStream},
	} {
		if got := storableType(c.contentType); got != c.want {
			t.Errorf("storableType() of %s = %q, want %q", name, got, c.want)
		}
	}
}
