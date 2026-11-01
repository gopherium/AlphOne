// SPDX-License-Identifier: Elastic-2.0

package message

import (
	"strings"
	"unicode/utf8"
)

// cleanText answers text without NUL bytes and with each run of broken UTF-8 turned into one replacement character.
func cleanText(text string) string {
	return strings.ToValidUTF8(strings.ReplaceAll(text, "\x00", ""), string(utf8.RuneError))
}
