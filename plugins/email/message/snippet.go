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

// ownText answers the lines the writer of text wrote, up to the first quote, forward or signature.
func ownText(text string) string {
	lines := strings.Split(strings.ReplaceAll(text, "\r\n", "\n"), "\n")
	var own []string
	wrote := false
	for i := 0; i < len(lines); i++ {
		line, next := lines[i], first(lines[i+1:])
		if span := attributionLines(line, next); span > 0 && !wrote {
			i += span - 1
			continue
		}
		if endsOwnText(line, next) {
			break
		}
		if !isQuoted(line) {
			own = append(own, line)
			wrote = wrote || strings.TrimSpace(line) != ""
		}
	}
	return strings.Join(own, "\n")
}

// attributionLines answers how many lines from line on hold an attribution such as On a date, someone wrote.
func attributionLines(line, next string) int {
	line = strings.TrimSpace(line)
	switch {
	case !strings.HasPrefix(line, "On ") || !strings.ContainsAny(line, "0123456789"):
		return 0
	case strings.HasSuffix(line, "wrote:"):
		return 1
	case strings.HasSuffix(strings.TrimSpace(next), "wrote:"):
		return 2
	}
	return 0
}

// endsOwnText reports whether line, followed by next, opens a quote, a forward or a signature.
func endsOwnText(line, next string) bool {
	marker := strings.ToLower(strings.Trim(line, "- \t"))
	return attributionLines(line, next) > 0 || line == "-- " || slices.Contains(ownTextEnds, marker) ||
		opensHeaderBlock(line, next)
}

// opensHeaderBlock reports whether line is a rule of underscores and next opens the header block Outlook quotes.
func opensHeaderBlock(line, next string) bool {
	rule := strings.TrimSpace(line)
	return rule != "" && strings.Trim(rule, "_") == "" && strings.HasPrefix(strings.TrimSpace(next), "From:")
}

// isQuoted reports whether line is a quoted line.
func isQuoted(line string) bool {
	return strings.HasPrefix(strings.TrimLeft(line, " \t"), ">")
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
