// SPDX-License-Identifier: Elastic-2.0

package message_test

import (
	"slices"
	"strings"
	"testing"
	"unicode"
	"unicode/utf8"

	"github.com/gopherium/alphone/plugins/email/message"
)

// parseIDsCases lists header values and the ids ParseIDs answers for each.
var parseIDsCases = []struct {
	name  string
	value string
	want  []string
}{
	{"one bracketed id", "<a@example.com>", []string{"a@example.com"}},
	{
		"two ids folded over a line break and a tab", "<a@example.com>\r\n\t<b@example.com>",
		[]string{"a@example.com", "b@example.com"},
	},
	{"two ids with no blank between", "<a@example.com><b@example.com>", []string{"a@example.com", "b@example.com"}},
	{"comments around an id", "(note) <a@example.com> (more)", []string{"a@example.com"}},
	{"a comment holding an id", "(see <c@example.com>) <a@example.com>", []string{"a@example.com"}},
	{"a nested comment", "(note (inner) <c@example.com>) <a@example.com>", []string{"a@example.com"}},
	{"an escaped parenthesis in a comment", `(note \) <c@example.com>) <a@example.com>`, []string{"a@example.com"}},
	{"an unclosed comment", "<a@example.com> (note <b@example.com>", []string{"a@example.com"}},
	{
		"words between ids", "<a@example.com> the note of <b@example.com>",
		[]string{"a@example.com", "b@example.com"},
	},
	{
		"a quoted phrase between ids", `<a@example.com> "the note <c@example.com>" <b@example.com>`,
		[]string{"a@example.com", "b@example.com"},
	},
	{"an escaped quote in a quoted phrase", `"say \" <c@example.com>" <a@example.com>`, []string{"a@example.com"}},
	{"an unclosed quoted phrase", `<a@example.com> "note <b@example.com>`, []string{"a@example.com"}},
	{"a bare id with no brackets", "a@example.com", []string{"a@example.com"}},
	{"bare ids split by a comma", "a@example.com,b@example.com", []string{"a@example.com", "b@example.com"}},
	{"bracketed ids split by a comma", "<a@example.com>, <b@example.com>", []string{"a@example.com", "b@example.com"}},
	{"the same id twice", "<a@example.com> <b@example.com> <a@example.com>", []string{"a@example.com", "b@example.com"}},
	{"empty brackets", "<>", nil},
	{"a blank between empty brackets", "< >", nil},
	{"a literal domain", "<a@[192.0.2.1]>", []string{"a@[192.0.2.1]"}},
	{"mixed case kept", "<A@Example.com>", []string{"A@Example.com"}},
	{"a bracketed id with no at sign", "<a.example.com>", []string{"a.example.com"}},
	{"a bare word with no at sign", "a.example.com", nil},
	{"an unclosed bracket at the end", "<a@example.com> <b@example.com", []string{"a@example.com"}},
	{"an unclosed bracket before an id", "<c@example.com <a@example.com>", []string{"a@example.com"}},
	{"a stray closing bracket", "> <a@example.com>", []string{"a@example.com"}},
	{"a blank inside the brackets", "<a @example.com>", nil},
	{"a control character inside the brackets", "<a\x00@example.com>", nil},
	{"invalid UTF-8 inside the brackets", "<a\xff@example.com>", nil},
	{"an id at the line limit", "<" + addressOfBytes(998) + ">", []string{addressOfBytes(998)}},
	{"an id over the line limit", "<" + addressOfBytes(999) + ">", nil},
	{"a bare id over the line limit", addressOfBytes(999), nil},
	{"a quoted left side", `<"01KF8"@example.com>`, []string{"01KF8@example.com"}},
	{"a quoted left side holding a blank", `<"a b"@example.com>`, nil},
	{"a quoted left side that needs its quotes", `<"a,b"@example.com>`, []string{`"a,b"@example.com`}},
	{"a quoted left side holding an at sign", `<"a@b"@example.com>`, []string{`"a@b"@example.com`}},
	{"a quoted left side holding a colon", `<"a:b"@example.com>`, []string{`"a:b"@example.com`}},
	{"a quoted left side holding a semicolon", `<"a;b"@example.com>`, []string{`"a;b"@example.com`}},
	{"a quoted left side holding a parenthesis", `<"a(b)"@example.com>`, []string{`"a(b)"@example.com`}},
	{"a quoted left side holding a square bracket", `<"a[b]"@example.com>`, []string{`"a[b]"@example.com`}},
	{"a quoted left side holding a backslash", `<"a\b"@example.com>`, []string{`"a\b"@example.com`}},
	{"an empty quoted left side", `<""@example.com>`, []string{`""@example.com`}},
	{"a bare quoted left side", `"abc"@example.com`, nil},
	{"two bare quoted left sides", `"x"@example.com "y"@example.com`, nil},
	{"a bare word opening with an at sign", "@example.com", nil},
	{"a quoted left side with dots", `<"a.b.c"@example.com>`, []string{"a.b.c@example.com"}},
	{"a quoted left side holding a quote", `<"a"b"@example.com>`, []string{`"a"b"@example.com`}},
	{"a quoted word with no domain", `<"01KF8">`, []string{`"01KF8"`}},
	{"a quote on one side only", `<01KF8"@example.com>`, []string{`01KF8"@example.com`}},
	{"empty", "", nil},
}

func TestParseIDs(t *testing.T) {
	t.Parallel()

	for _, c := range parseIDsCases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()

			if got := message.ParseIDs(c.value); !slices.Equal(got, c.want) {
				t.Errorf("ParseIDs(%q) = %q, want %q", c.value, got, c.want)
			}
		})
	}
}

func TestFormatIDs(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		ids  []string
		want string
	}{
		{"no ids", nil, ""},
		{"one id", []string{"a@example.com"}, "<a@example.com>"},
		{"two ids", []string{"a@example.com", "b@example.com"}, "<a@example.com> <b@example.com>"},
		{
			"an id holding a line break",
			[]string{"a@example.com>\r\nBcc: x@example.com\r\nX: <b@example.com", "c@example.com"},
			"<c@example.com>",
		},
		{"an id holding a bracket", []string{"a@example.com> <b@example.com", "c@example.com"}, "<c@example.com>"},
		{"an id holding a blank", []string{"a @example.com", "c@example.com"}, "<c@example.com>"},
		{"an id holding invalid UTF-8", []string{"a\xff@example.com", "c@example.com"}, "<c@example.com>"},
		{"an id over the line limit", []string{addressOfBytes(999), "c@example.com"}, "<c@example.com>"},
		{"an empty id", []string{"", "c@example.com"}, "<c@example.com>"},
		{"only unclean ids", []string{"a @example.com", "<b@example.com>"}, ""},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()

			if got := message.FormatIDs(c.ids); got != c.want {
				t.Errorf("FormatIDs(%q) = %q, want %q", c.ids, got, c.want)
			}
		})
	}
}

func FuzzParseIDsAnswersCleanUniqueIDs(f *testing.F) {
	for _, c := range parseIDsCases {
		f.Add(c.value)
	}
	f.Fuzz(func(t *testing.T, value string) {
		ids := message.ParseIDs(value)
		seen := make(map[string]bool, len(ids))
		for _, id := range ids {
			if !isCleanID(id) {
				t.Fatalf("ParseIDs(%q) answered %q, want a non-empty UTF-8 id with no bracket, blank or control", value, id)
			}
			if seen[id] {
				t.Fatalf("ParseIDs(%q) answered %q twice", value, id)
			}
			seen[id] = true
		}
		if again := message.ParseIDs(message.FormatIDs(ids)); !slices.Equal(again, ids) {
			t.Fatalf("ParseIDs(FormatIDs(%q)) = %q, want the same ids", ids, again)
		}
	})
}

// isCleanID reports whether id is non-empty UTF-8 within the line limit holding no bracket, blank or control character.
func isCleanID(id string) bool {
	return id != "" && len(id) <= 998 && utf8.ValidString(id) && !strings.ContainsAny(id, "<>") &&
		!strings.ContainsFunc(id, unicode.IsSpace) && !strings.ContainsFunc(id, unicode.IsControl)
}
