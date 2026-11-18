// SPDX-License-Identifier: Elastic-2.0

package message

import (
	"fmt"
	"slices"
	"testing"
)

// scannedToken is one token of a style value as a test writes it.
type scannedToken struct {
	// kind is what the style reader needs to know of the token.
	kind styleTokenKind
	// text is the part of the style value the token covers.
	text string
}

// String answers the kind and the quoted text of the token.
func (t scannedToken) String() string {
	return fmt.Sprintf("%d %q", t.kind, t.text)
}

// scanned answers every token the style scanner cuts text into, up to the end or the first error.
func scanned(text string) []scannedToken {
	scanner := styleScanner{text: text}
	var tokens []scannedToken
	for {
		token := scanner.next()
		tokens = append(tokens, scannedToken{kind: token.kind, text: text[token.start:token.end]})
		if token.kind == tokenEnd || token.kind == tokenError {
			return tokens
		}
	}
}

// tokenCases lists style values with the tokens the style scanner cuts each into, covering every kind of token.
var tokenCases = []struct {
	name string
	text string
	want []scannedToken
}{
	{
		"a byte order mark at the start",
		"\U0000FEFFa",
		[]scannedToken{{tokenText, "\U0000FEFF"}, {tokenText, "a"}, {tokenEnd, ""}},
	},
	{
		"a byte order mark after the start",
		"a\U0000FEFF",
		[]scannedToken{{tokenText, "a\U0000FEFF"}, {tokenEnd, ""}},
	},
	{
		"colon, semicolon and braces",
		":;{}",
		[]scannedToken{{tokenColon, ":"}, {tokenSemicolon, ";"}, {tokenOpen, "{"}, {tokenClose, "}"}, {tokenEnd, ""}},
	},
	{
		"characters on their own",
		",%&+=>()[]",
		[]scannedToken{
			{tokenText, ","}, {tokenText, "%"}, {tokenText, "&"}, {tokenText, "+"}, {tokenText, "="},
			{tokenText, ">"}, {tokenText, "("}, {tokenText, ")"}, {tokenText, "["}, {tokenText, "]"}, {tokenEnd, ""},
		},
	},
	{
		"white space",
		" \t\na",
		[]scannedToken{{tokenSkipped, " \t\n"}, {tokenText, "a"}, {tokenEnd, ""}},
	},
	{
		"double and single quoted strings",
		"\"a b\"'c'",
		[]scannedToken{{tokenText, "\"a b\""}, {tokenText, "'c'"}, {tokenEnd, ""}},
	},
	{"an unclosed string", "'a", []scannedToken{{tokenError, ""}}},
	{"a string holding a tab", "'a\tb'", []scannedToken{{tokenError, ""}}},
	{"a string holding U+FFFE", "'\U0000FFFE'", []scannedToken{{tokenError, ""}}},
	{"a string holding U+FFFF", "'\U0000FFFF'", []scannedToken{{tokenError, ""}}},
	{
		"a string holding a letter past ASCII and a broken byte",
		"'\U000000E9\xff'",
		[]scannedToken{{tokenText, "'\U000000E9\xff'"}, {tokenEnd, ""}},
	},
	{
		"a comment",
		"/*a*/b",
		[]scannedToken{{tokenSkipped, "/*a*/"}, {tokenText, "b"}, {tokenEnd, ""}},
	},
	{"an unclosed comment", "/*a", []scannedToken{{tokenError, ""}}},
	{
		"a slash on its own",
		"/a",
		[]scannedToken{{tokenText, "/"}, {tokenText, "a"}, {tokenEnd, ""}},
	},
	{
		"a hash sign with a name and on its own",
		"#a-1 #",
		[]scannedToken{{tokenText, "#a-1"}, {tokenSkipped, " "}, {tokenText, "#"}, {tokenEnd, ""}},
	},
	{
		"an at sign with an identifier and on its own",
		"@media @1",
		[]scannedToken{
			{tokenText, "@media"}, {tokenSkipped, " "}, {tokenText, "@"}, {tokenText, "1"}, {tokenEnd, ""},
		},
	},
	{
		"an opening comment mark and a less-than sign",
		"<!--a<b",
		[]scannedToken{
			{tokenSkipped, "<!--"}, {tokenText, "a"}, {tokenText, "<"}, {tokenText, "b"}, {tokenEnd, ""},
		},
	},
	{
		"full stops before a digit, before a letter and at the end",
		".5em .a .",
		[]scannedToken{
			{tokenText, ".5em"}, {tokenSkipped, " "}, {tokenText, "."}, {tokenText, "a"}, {tokenSkipped, " "},
			{tokenText, "."}, {tokenEnd, ""},
		},
	},
	{
		"attribute matchers and their first characters",
		"~=~|=|^=^$=$*=*",
		[]scannedToken{
			{tokenText, "~="}, {tokenText, "~"}, {tokenText, "|="}, {tokenText, "|"}, {tokenText, "^="},
			{tokenText, "^"}, {tokenText, "$="}, {tokenText, "$"}, {tokenText, "*="}, {tokenText, "*"}, {tokenEnd, ""},
		},
	},
	{
		"bare and quoted urls",
		"url(a)url( 'b' )url(\"c\")",
		[]scannedToken{{tokenText, "url(a)"}, {tokenText, "url( 'b' )"}, {tokenText, "url(\"c\")"}, {tokenEnd, ""}},
	},
	{
		"a quoted url with no closing parenthesis after its string",
		"url('a'x)",
		[]scannedToken{{tokenText, "url("}, {tokenText, "'a'"}, {tokenText, "x"}, {tokenText, ")"}, {tokenEnd, ""}},
	},
	{
		"a url with an unclosed string",
		"url('a",
		[]scannedToken{{tokenText, "url("}, {tokenError, ""}},
	},
	{
		"a bare url broken by a blank",
		"url(a b)",
		[]scannedToken{
			{tokenText, "url("}, {tokenText, "a"}, {tokenSkipped, " "}, {tokenText, "b"}, {tokenText, ")"},
			{tokenEnd, ""},
		},
	},
	{
		"a bare url running to the end",
		"url(a",
		[]scannedToken{{tokenText, "url("}, {tokenText, "a"}, {tokenEnd, ""}},
	},
	{
		"urls in upper and mixed case",
		"URL(a)Url(a)",
		[]scannedToken{
			{tokenText, "URL("}, {tokenText, "a"}, {tokenText, ")"}, {tokenText, "Url("}, {tokenText, "a"},
			{tokenText, ")"}, {tokenEnd, ""},
		},
	},
	{
		"a function",
		"rgb(1,2,3)",
		[]scannedToken{
			{tokenText, "rgb("}, {tokenText, "1"}, {tokenText, ","}, {tokenText, "2"}, {tokenText, ","},
			{tokenText, "3"}, {tokenText, ")"}, {tokenEnd, ""},
		},
	},
	{
		"unicode ranges",
		"U+0-7F U+00?? U+1- U+x U+a U+1234567",
		[]scannedToken{
			{tokenText, "U+0-7F"}, {tokenSkipped, " "}, {tokenText, "U+00??"}, {tokenSkipped, " "},
			{tokenText, "U+1"}, {tokenText, "-"}, {tokenSkipped, " "}, {tokenText, "U"}, {tokenText, "+"},
			{tokenText, "x"}, {tokenSkipped, " "}, {tokenText, "U"}, {tokenText, "+"}, {tokenText, "a"},
			{tokenSkipped, " "}, {tokenText, "U+123456"}, {tokenText, "7"}, {tokenEnd, ""},
		},
	},
	{
		"identifiers",
		"-a a-b_1 \U000000E9 \xff",
		[]scannedToken{
			{tokenText, "-a"}, {tokenSkipped, " "}, {tokenText, "a-b_1"}, {tokenSkipped, " "},
			{tokenText, "\U000000E9"}, {tokenSkipped, " "}, {tokenText, "\xff"}, {tokenEnd, ""},
		},
	},
	{
		"dimensions, percentages and numbers",
		"5px 5% 5 5.5 5. .5% -5",
		[]scannedToken{
			{tokenText, "5px"}, {tokenSkipped, " "}, {tokenText, "5%"}, {tokenSkipped, " "}, {tokenText, "5"},
			{tokenSkipped, " "}, {tokenText, "5.5"}, {tokenSkipped, " "}, {tokenText, "5"}, {tokenText, "."},
			{tokenSkipped, " "}, {tokenText, ".5%"}, {tokenSkipped, " "}, {tokenText, "-"}, {tokenText, "5"},
			{tokenEnd, ""},
		},
	},
	{
		"a closing comment mark and a minus on its own",
		"-->->",
		[]scannedToken{{tokenSkipped, "-->"}, {tokenText, "-"}, {tokenText, ">"}, {tokenEnd, ""}},
	},
	{
		"characters with no rule of their own",
		"\v\x01\U0000FFFE!",
		[]scannedToken{
			{tokenText, "\v"}, {tokenText, "\x01"}, {tokenText, "\U0000FFFE"}, {tokenText, "!"}, {tokenEnd, ""},
		},
	},
	{"nothing", "", []scannedToken{{tokenEnd, ""}}},
}

func TestStyleScannerCutsEachKindOfToken(t *testing.T) {
	t.Parallel()

	for _, c := range tokenCases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()

			if got := scanned(c.text); !slices.Equal(got, c.want) {
				t.Errorf("scanned(%q) = %v, want %v", c.text, got, c.want)
			}
		})
	}
}

func TestStyleScannerFailsAUrlInsideAFailedRunAtOnce(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		text string
		want []scannedToken
	}{
		{
			"urls inside the run of one that does not close",
			"url(url(url(x",
			[]scannedToken{{tokenText, "url("}, {tokenText, "url("}, {tokenText, "url("}, {tokenText, "x"}, {tokenEnd, ""}},
		},
		{
			"a quoted url where the run of one that does not close stops",
			"url(url( 'a')",
			[]scannedToken{{tokenText, "url("}, {tokenText, "url( 'a')"}, {tokenEnd, ""}},
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()

			if got := scanned(c.text); !slices.Equal(got, c.want) {
				t.Errorf("scanned(%q) = %v, want %v", c.text, got, c.want)
			}
		})
	}
}

func TestWithoutImportantCutsOnlyTheLastFlag(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name  string
		value string
		want  string
	}{
		{"a value shorter than the flag", "red", "red"},
		{"a value with no flag", "lightsteelblue", "lightsteelblue"},
		{"a plain flag", "red !important", "red"},
		{"an upper case flag", "red !IMPORTANT", "red"},
		{"a tab before the flag", "red\t!important", "red"},
		{"white space after the flag", "red !important \n", "red"},
		{"a no-break space after the flag", "red !important\U000000A0", "red !important\U000000A0"},
		{"two flags", "red !important !important", "red !important"},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()

			if got := withoutImportant(c.value); got != c.want {
				t.Errorf("withoutImportant(%q) = %q, want %q", c.value, got, c.want)
			}
		})
	}
}
