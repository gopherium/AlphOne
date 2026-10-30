// SPDX-License-Identifier: Elastic-2.0

package message

import (
	"strings"
	"unicode"
	"unicode/utf8"
)

// wordEnds lists the bytes that end a bare word in a message id list.
const wordEnds = " \t\r\n<>()\","

// ParseIDs answers the message ids in a Message-ID, In-Reply-To or References value, unbracketed and each once.
func ParseIDs(value string) []string {
	var ids []string
	seen := make(map[string]bool)
	for rest := value; rest != ""; {
		var id string
		id, rest = nextID(rest)
		if id != "" && !seen[id] {
			seen[id] = true
			ids = append(ids, id)
		}
	}
	return ids
}

// FormatIDs answers the clean ids among ids bracketed and joined by single blanks.
func FormatIDs(ids []string) string {
	kept := cleanIDs(ids)
	if len(kept) == 0 {
		return ""
	}
	return "<" + strings.Join(kept, "> <") + ">"
}

// cleanIDs answers the ids that are UTF-8 with no bracket, blank or control character, in order.
func cleanIDs(ids []string) []string {
	var kept []string
	for _, id := range ids {
		if cleanID(id) != "" && !strings.ContainsAny(id, "<>") {
			kept = append(kept, id)
		}
	}
	return kept
}

// nextID reads the token opening s, answering the id it holds, empty for none, and the text after it.
func nextID(s string) (string, string) {
	switch s[0] {
	case '(':
		return "", afterComment(s)
	case '"':
		return "", afterQuoted(s)
	case '<':
		return bracketed(s)
	}
	end := strings.IndexAny(s, wordEnds)
	if end == 0 {
		return "", s[1:]
	}
	if end < 0 {
		end = len(s)
	}
	return bareID(s[:end]), s[end:]
}

// bracketed reads the id inside the bracket opening s, answering it, empty for none, and the text after it.
func bracketed(s string) (string, string) {
	end := strings.IndexAny(s[1:], "<>") + 1
	if end == 0 {
		return "", ""
	}
	if s[end] == '<' {
		return "", s[end:]
	}
	return cleanID(s[1:end]), s[end+1:]
}

// bareID answers word when it reads as an id without brackets, empty otherwise.
func bareID(word string) string {
	if !strings.Contains(word, "@") {
		return ""
	}
	return cleanID(word)
}

// cleanID answers id when it is UTF-8 with no blank or control character, empty otherwise.
func cleanID(id string) string {
	if !utf8.ValidString(id) || strings.ContainsFunc(id, isBlankOrControl) {
		return ""
	}
	return id
}

// isBlankOrControl reports whether r is a blank or a control character.
func isBlankOrControl(r rune) bool {
	return unicode.IsSpace(r) || unicode.IsControl(r)
}

// afterComment answers the text after the comment opening s, empty when it never closes.
func afterComment(s string) string {
	depth := 0
	for i := 0; i < len(s); i++ {
		switch s[i] {
		case '\\':
			i++
		case '(':
			depth++
		case ')':
			depth--
			if depth == 0 {
				return s[i+1:]
			}
		}
	}
	return ""
}

// afterQuoted answers the text after the quoted phrase opening s, empty when it never closes.
func afterQuoted(s string) string {
	for i := 1; i < len(s); i++ {
		switch s[i] {
		case '\\':
			i++
		case '"':
			return s[i+1:]
		}
	}
	return ""
}
