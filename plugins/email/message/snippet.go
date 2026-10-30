// SPDX-License-Identifier: Elastic-2.0

package message

import (
	"cmp"
	"slices"
	"strings"
	"unicode/utf8"
)

// ownTextEnds lists the lines that open a quoted message, a forwarded message or a mobile signature.
var ownTextEnds = []string{
	"original message", "forwarded message", "begin forwarded message:",
	"sent from my iphone", "sent from my ipad", "get outlook for ios", "get outlook for android",
}

// Snippet answers a preview of text at most limit letters long, without quoted replies or signatures.
func Snippet(text string, limit int) (string, error) {
	if limit <= 0 {
		return "", ErrLimitsUnset
	}
	return cmp.Or(clip(ownText(text), limit), clip(text, limit)), nil
}

// replyLine is a line of a reply with the line after it.
type replyLine struct {
	text string
	next string
}

// ownText answers the lines the writer of text wrote, up to the first quote, forward or signature.
func ownText(text string) string {
	lines := strings.Split(strings.ReplaceAll(text, "\r\n", "\n"), "\n")
	var own []string
	wrote := false
	for i := 0; i < len(lines); i++ {
		line := replyLine{lines[i], first(lines[i+1:])}
		if span := line.attribution(); span > 0 && !wrote {
			i += span - 1
			continue
		}
		if line.endsOwnText() {
			break
		}
		if !line.quoted() {
			own = append(own, line.text)
			wrote = wrote || strings.TrimSpace(line.text) != ""
		}
	}
	return strings.Join(own, "\n")
}

// attribution answers how many lines from l on hold an attribution such as On a date, someone wrote.
func (l replyLine) attribution() int {
	text := strings.TrimSpace(l.text)
	switch {
	case !strings.HasPrefix(text, "On ") || !strings.ContainsAny(text, "0123456789"):
		return 0
	case strings.HasSuffix(text, "wrote:"):
		return 1
	case strings.HasSuffix(strings.TrimSpace(l.next), "wrote:"):
		return 2
	}
	return 0
}

// endsOwnText reports whether l opens a quote, a forward or a signature.
func (l replyLine) endsOwnText() bool {
	marker := strings.ToLower(strings.Trim(l.text, "- \t"))
	return l.attribution() > 0 || l.text == "-- " || slices.Contains(ownTextEnds, marker) || l.opensHeaderBlock()
}

// opensHeaderBlock reports whether l is a rule of underscores followed by the header block Outlook quotes.
func (l replyLine) opensHeaderBlock() bool {
	rule := strings.TrimSpace(l.text)
	return rule != "" && strings.Trim(rule, "_") == "" && strings.HasPrefix(strings.TrimSpace(l.next), "From:")
}

// quoted reports whether l is a quoted line.
func (l replyLine) quoted() bool {
	return strings.HasPrefix(strings.TrimLeft(l.text, " \t"), ">")
}

// clip answers the words of text joined by single blanks, cut to at most limit letters.
func clip(text string, limit int) string {
	var words []string
	letters := -1
	for word := range strings.FieldsSeq(text) {
		words = append(words, word)
		letters += 1 + utf8.RuneCountInString(word)
		if letters >= limit {
			break
		}
	}
	return cut(strings.Join(words, " "), limit)
}

// cut answers the first limit letters of preview without a trailing blank.
func cut(preview string, limit int) string {
	letters := 0
	for i := range preview {
		if letters == limit {
			return strings.TrimRight(preview[:i], " ")
		}
		letters++
	}
	return preview
}
