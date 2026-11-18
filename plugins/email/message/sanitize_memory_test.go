// SPDX-License-Identifier: Elastic-2.0

//go:build !race

package message_test

import (
	"strings"
	"testing"

	"github.com/gopherium/alphone/plugins/email/message"
)

func TestSanitizeKeepsALongStyleValueInsideASmallMultipleOfItsSize(t *testing.T) {
	cases := []struct {
		name   string
		prefix string
		unit   string
	}{
		{"font names apart", "font-family: ", "a "},
		{"a list of font names", "font-family: ", "a,"},
		{"quoted font names", "font-family: ", "'a' "},
		{"comments in a font family", "font-family: ", "/**/"},
		{"words with no colon", "", "a "},
		{"a colour of commas", "color: ", ","},
		{"an alignment of commas", "text-align: ", ","},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			html := `<p style="` + c.prefix + strings.Repeat(c.unit, 64<<10/len(c.unit)) + `">Lamp</p>`
			checkSanitizeAllocation(t, c.name, html)
		})
	}
}

func TestSanitizeKeepsManyStyledElementsInsideASmallMultipleOfTheirSize(t *testing.T) {
	cases := []struct {
		name    string
		element string
	}{
		{"styled spans", `<span style="color: red">Lamp</span>`},
		{"styled table cells", `<td style="padding-top: 4px">Lamp</td>`},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			checkSanitizeAllocation(t, c.name, strings.Repeat(c.element, 64<<10/len(c.element)))
		})
	}
}

// checkSanitizeAllocation fails t when Sanitize of html, described by name, allocates over 64 times its size.
func checkSanitizeAllocation(t *testing.T, name, html string) {
	t.Helper()
	allocated := allocatedBy(func() { message.Sanitize(html, wideLimit) })

	if most := uint64(64 * len(html)); allocated > most {
		t.Errorf("Sanitize() of %d bytes of %s allocated %d KiB, want at most %d KiB", len(html), name, allocated>>10,
			most>>10)
	}
}
