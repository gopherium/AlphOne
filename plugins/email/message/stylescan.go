// SPDX-License-Identifier: Elastic-2.0

package message

import (
	"strings"
	"unicode/utf8"
)

// styleTokenKind is what the style reader needs to know of a token.
type styleTokenKind int

const (
	// tokenText is a token the style reader only keeps as text.
	tokenText styleTokenKind = iota
	// tokenSkipped is white space, a comment, "<!--" or "-->", skipped before a declaration.
	tokenSkipped
	// tokenColon is a colon on its own.
	tokenColon
	// tokenSemicolon is a semicolon on its own.
	tokenSemicolon
	// tokenOpen is an opening brace on its own.
	tokenOpen
	// tokenClose is a closing brace on its own.
	tokenClose
	// tokenEnd is the end of the style value.
	tokenEnd
	// tokenError is a string or a comment that does not close, or a string holding a character it may not.
	tokenError
)

// styleToken is one token of a style value.
type styleToken struct {
	// kind is what the style reader needs to know of the token.
	kind styleTokenKind
	// start is the offset of the token's first byte.
	start int
	// end is the offset after the token's last byte.
	end int
}

// runeClass is a set of the character classes of the style scanner.
type runeClass uint8

const (
	// classNameStart is a character an identifier may start with.
	classNameStart runeClass = 1 << iota
	// classNameChar is a character an identifier may hold.
	classNameChar
	// classURL is a character an unquoted url( address may hold.
	classURL
	// classString is a character a quoted string may hold besides its own quote.
	classString
	// classBlank is white space.
	classBlank
	// classDigit is an ASCII digit.
	classDigit
)

// classNonASCII is the class of every rune past ASCII but U+FFFE and U+FFFF, a broken byte included.
const classNonASCII = classNameStart | classNameChar | classURL | classString

// byteOrderMark is U+FEFF, a token of its own only at the start of a style value.
const byteOrderMark = "\xef\xbb\xbf"

// upperHex lists the digits of a unicode range.
const upperHex = "0123456789ABCDEF"

// asciiClasses holds the classes of each ASCII byte.
var asciiClasses = asciiClassTable()

// singleTokens maps each character the style reader needs to know of on its own to its kind.
var singleTokens = map[byte]styleTokenKind{':': tokenColon, ';': tokenSemicolon, '{': tokenOpen, '}': tokenClose}

// leadRules maps each first byte with a rule of its own to that rule.
var leadRules = map[byte]func(*styleScanner) (styleTokenKind, int){
	'\t': (*styleScanner).whiteSpace, '\n': (*styleScanner).whiteSpace, ' ': (*styleScanner).whiteSpace,
	'"': (*styleScanner).quoted, '\'': (*styleScanner).quoted, '/': (*styleScanner).slash, '#': (*styleScanner).hash,
	'@': (*styleScanner).atKeyword, '<': (*styleScanner).openComment, '~': (*styleScanner).matcher,
	'|': (*styleScanner).matcher, '^': (*styleScanner).matcher, '$': (*styleScanner).matcher,
	'*': (*styleScanner).matcher,
}

// asciiClassTable answers the classes of each ASCII byte.
func asciiClassTable() [utf8.RuneSelf]runeClass {
	var table [utf8.RuneSelf]runeClass
	for b := '!'; b <= '~'; b++ {
		table[b] = classURL | classString
	}
	table['"'], table['\''], table['\\'], table[' '] = classString, classString, 0, classString
	for _, b := range "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ_" {
		table[b] |= classNameStart | classNameChar
	}
	for _, b := range "0123456789" {
		table[b] |= classNameChar | classDigit
	}
	table['-'] |= classNameChar
	for _, b := range blanks {
		table[b] |= classBlank
	}
	return table
}

// styleScanner cuts a style value into the tokens the style reader reads.
type styleScanner struct {
	// text is the style value, every line break a line feed.
	text string
	// pos is the offset of the next token.
	pos int
	// failedURL is the offset where the address run of the last url( that did not close stops.
	failedURL int
}

// next answers the token at pos and moves past it.
func (s *styleScanner) next() styleToken {
	start := s.pos
	kind, end := s.token()
	s.pos = end
	return styleToken{kind: kind, start: start, end: end}
}

// token answers the kind and the end of the token at pos.
func (s *styleScanner) token() (styleTokenKind, int) {
	if s.pos >= len(s.text) {
		return tokenEnd, s.pos
	}
	if s.pos == 0 && strings.HasPrefix(s.text, byteOrderMark) {
		return tokenText, len(byteOrderMark)
	}
	lead := s.text[s.pos]
	if kind, ok := singleTokens[lead]; ok {
		return kind, s.pos + 1
	}
	if rule, ok := leadRules[lead]; ok {
		return rule(s)
	}
	return s.pattern()
}

// pattern answers the kind and the end of a token led by any other byte.
func (s *styleScanner) pattern() (styleTokenKind, int) {
	if end := s.urlEnd(); end >= 0 {
		return tokenText, end
	}
	ident := s.identEnd(s.pos)
	if ident >= 0 && s.at(ident, '(') {
		return tokenText, ident + 1
	}
	if end := s.unicodeRangeEnd(); end >= 0 {
		return tokenText, end
	}
	if ident >= 0 {
		return tokenText, ident
	}
	if end := s.numericEnd(); end >= 0 {
		return tokenText, end
	}
	if strings.HasPrefix(s.text[s.pos:], "-->") {
		return tokenSkipped, s.pos + len("-->")
	}
	_, width := utf8.DecodeRuneInString(s.text[s.pos:])
	return tokenText, s.pos + width
}

// whiteSpace answers a run of white space.
func (s *styleScanner) whiteSpace() (styleTokenKind, int) {
	return tokenSkipped, s.runEnd(s.pos, classBlank)
}

// quoted answers a quoted string, or an error when it does not close.
func (s *styleScanner) quoted() (styleTokenKind, int) {
	if end := s.stringEnd(s.pos); end >= 0 {
		return tokenText, end
	}
	return tokenError, s.pos
}

// slash answers a comment, an error when it does not close, or a slash on its own.
func (s *styleScanner) slash() (styleTokenKind, int) {
	if !s.at(s.pos+1, '*') {
		return tokenText, s.pos + 1
	}
	closing := strings.Index(s.text[s.pos+2:], "*/")
	if closing < 0 {
		return tokenError, s.pos
	}
	return tokenSkipped, s.pos + 2 + closing + 2
}

// hash answers a hash sign and its name, or the hash sign on its own.
func (s *styleScanner) hash() (styleTokenKind, int) {
	return tokenText, max(s.runEnd(s.pos+1, classNameChar), s.pos+1)
}

// atKeyword answers an at sign and its identifier, or the at sign on its own.
func (s *styleScanner) atKeyword() (styleTokenKind, int) {
	return tokenText, max(s.identEnd(s.pos+1), s.pos+1)
}

// openComment answers "<!--", or the less-than sign on its own.
func (s *styleScanner) openComment() (styleTokenKind, int) {
	if strings.HasPrefix(s.text[s.pos:], "<!--") {
		return tokenSkipped, s.pos + len("<!--")
	}
	return tokenText, s.pos + 1
}

// matcher answers an attribute matcher such as "~=", or its first character on its own.
func (s *styleScanner) matcher() (styleTokenKind, int) {
	if s.at(s.pos+1, '=') {
		return tokenText, s.pos + 2
	}
	return tokenText, s.pos + 1
}

// urlEnd answers the end of a url( token at pos, or -1.
func (s *styleScanner) urlEnd() int {
	if !strings.HasPrefix(s.text[s.pos:], "url(") || s.pos+len("url(") < s.failedURL {
		return -1
	}
	address := s.runEnd(s.pos+len("url("), classBlank)
	if end := s.quotedURLEnd(address); end >= 0 {
		return end
	}
	return s.bareURLEnd(address)
}

// quotedURLEnd answers the end of a url( whose address is the string at i, or -1.
func (s *styleScanner) quotedURLEnd(i int) int {
	if !s.isQuote(i) {
		return -1
	}
	if end := s.stringEnd(i); end >= 0 {
		return s.closingParen(end)
	}
	return -1
}

// bareURLEnd answers the end of a url( whose unquoted address starts at i, or -1, noting where a failed run stops.
func (s *styleScanner) bareURLEnd(i int) int {
	for {
		if end := s.closingParen(i); end >= 0 {
			return end
		}
		class, width := s.classAt(i)
		if class&classURL == 0 {
			s.failedURL = i
			return -1
		}
		i += width
	}
}

// closingParen answers the end of white space then a closing parenthesis at i, or -1.
func (s *styleScanner) closingParen(i int) int {
	if end := s.runEnd(i, classBlank); s.at(end, ')') {
		return end + 1
	}
	return -1
}

// stringEnd answers the end of the string whose quote is at i, or -1 when it does not close.
func (s *styleScanner) stringEnd(i int) int {
	quote := s.text[i]
	for j := i + 1; j < len(s.text); {
		if s.text[j] == quote {
			return j + 1
		}
		class, width := s.classAt(j)
		if class&classString == 0 {
			return -1
		}
		j += width
	}
	return -1
}

// identEnd answers the end of an identifier at i, or -1.
func (s *styleScanner) identEnd(i int) int {
	if s.at(i, '-') {
		i++
	}
	class, width := s.classAt(i)
	if class&classNameStart == 0 {
		return -1
	}
	return s.runEnd(i+width, classNameChar)
}

// unicodeRangeEnd answers the end of a unicode range such as "U+0-7F" at pos, or -1.
func (s *styleScanner) unicodeRangeEnd() int {
	if !strings.HasPrefix(s.text[s.pos:], "U+") {
		return -1
	}
	first := s.hexEnd(s.pos+len("U+"), upperHex+"?")
	if first == s.pos+len("U+") {
		return -1
	}
	if second := s.hexEnd(first+1, upperHex); s.at(first, '-') && second > first+1 {
		return second
	}
	return first
}

// hexEnd answers the end of at most six bytes of digits at i.
func (s *styleScanner) hexEnd(i int, digits string) int {
	end := i
	for end < min(i+6, len(s.text)) && strings.IndexByte(digits, s.text[end]) >= 0 {
		end++
	}
	return end
}

// numericEnd answers the end of a dimension, a percentage or a number at pos, or -1.
func (s *styleScanner) numericEnd() int {
	number := s.numberEnd()
	if number < 0 {
		return -1
	}
	if unit := s.identEnd(number); unit >= 0 {
		return unit
	}
	if s.at(number, '%') {
		return number + 1
	}
	return number
}

// numberEnd answers the end of a number at pos, its decimal form first, or -1.
func (s *styleScanner) numberEnd() int {
	whole := s.runEnd(s.pos, classDigit)
	if s.at(whole, '.') && s.has(whole+1, classDigit) {
		return s.runEnd(whole+1, classDigit)
	}
	if whole > s.pos {
		return whole
	}
	return -1
}

// isQuote reports whether the byte at i is a quotation mark.
func (s *styleScanner) isQuote(i int) bool {
	return s.at(i, '"') || s.at(i, '\'')
}

// at reports whether the byte at i is c.
func (s *styleScanner) at(i int, c byte) bool {
	return i < len(s.text) && s.text[i] == c
}

// has reports whether the rune at i is in class.
func (s *styleScanner) has(i int, class runeClass) bool {
	got, _ := s.classAt(i)
	return got&class != 0
}

// runEnd answers the end of the run of runes in class at i.
func (s *styleScanner) runEnd(i int, class runeClass) int {
	for {
		got, width := s.classAt(i)
		if got&class == 0 {
			return i
		}
		i += width
	}
}

// classAt answers the classes of the rune at i and its width, no class past the end.
func (s *styleScanner) classAt(i int) (runeClass, int) {
	if i >= len(s.text) {
		return 0, 0
	}
	if b := s.text[i]; b < utf8.RuneSelf {
		return asciiClasses[b], 1
	}
	r, width := utf8.DecodeRuneInString(s.text[i:])
	if r == 0xFFFE || r == 0xFFFF {
		return 0, width
	}
	return classNonASCII, width
}
