// SPDX-License-Identifier: Elastic-2.0

//go:build !race

package message_test

import (
	"strings"
	"testing"
)

func TestParseKeepsAnHTMLMailWithALongStyleInsideASmallMultipleOfItsSize(t *testing.T) {
	cases := []struct {
		name string
		raw  string
	}{
		{"one long kept list of quoted font names", oneStyledParagraph("font-family: ", "'a',", "'a'")},
		{"one long kept colour list", oneStyledParagraph("color: ", "red,", "red")},
		{"one long kept list of widths", oneStyledParagraph("width: ", "auto,", "auto")},
		{"one long run of colour declarations", oneStyledParagraph("", "color:red;", "")},
		{"one long colour value of commas", oneStyledParagraph("color: ", ",", "")},
		{
			"one long font family of letters a meta charset widens",
			strings.Replace(oneStyledParagraph("font-family: ", "\xa1", ""), "<p style",
				`<meta charset="windows-874"><p style`, 1),
		},
		{"one long colour value of url openings", oneStyledParagraph("color: ", "url(", "")},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			raw := []byte(c.raw)

			if allocated := allocatedBy(func() { readWholeMail(t, raw) }); allocated > htmlMemoryBudget {
				t.Errorf("reading %d bytes of HTML allocated %d MiB, want at most %d MiB", len(raw), allocated>>20,
					htmlMemoryBudget>>20)
			}
		})
	}
}

// oneStyledParagraph answers an HTML mail of a paragraph styled prefix, unit up to the body cap and last, then filler.
func oneStyledParagraph(prefix, unit, last string) string {
	style := `<p style="` + prefix + strings.Repeat(unit, (serverBodyCap-64-len(prefix)-len(last))/len(unit)) + last +
		`">Lamp</p>`
	return "Content-Type: text/html\r\n\r\n" + style + "\r\n" + filled("Lamp ")[:mailBytes-len(style)]
}
