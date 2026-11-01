// SPDX-License-Identifier: Elastic-2.0

package message_test

import (
	"net/mail"
	"slices"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/gopherium/alphone/plugins/email/message"
)

// replacement is the letter that stands in for a run of broken UTF-8.
const replacement = string(utf8.RuneError)

// normalizeAddressCases lists mailboxes and what NormalizeAddress answers for each.
var normalizeAddressCases = []struct {
	name  string
	input mail.Address
	want  message.Address
	ok    bool
}{
	{
		"a mixed case email", mail.Address{Name: "Maria Perez", Address: "Maria.Perez@Example.COM"},
		message.Address{Name: "Maria Perez", Email: "maria.perez@example.com"}, true,
	},
	{
		"blanks around the name and the email", mail.Address{Name: " Maria Perez ", Address: "\tmaria.perez@example.com "},
		message.Address{Name: "Maria Perez", Email: "maria.perez@example.com"}, true,
	},
	{"no name", mail.Address{Address: "info@example.com"}, message.Address{Email: "info@example.com"}, true},
	{"an empty email", mail.Address{Name: "Maria Perez"}, message.Address{}, false},
	{"a blank email", mail.Address{Name: "Maria Perez", Address: "  "}, message.Address{}, false},
	{
		"an email at the length limit", mail.Address{Address: addressOfBytes(254)},
		message.Address{Email: addressOfBytes(254)}, true,
	},
	{"an email over the length limit", mail.Address{Address: addressOfBytes(255)}, message.Address{}, false},
	{"an email holding a NUL", mail.Address{Address: "a\x00b@example.com"}, message.Address{}, false},
	{"an email holding a control character", mail.Address{Address: "a\x1bb@example.com"}, message.Address{}, false},
	{"an email holding broken UTF-8", mail.Address{Address: "a\xffb@example.com"}, message.Address{}, false},
	{
		"an email growing over the limit when lowercased",
		mail.Address{Address: strings.Repeat(string(rune(0x23a)), 121) + "@example.com"}, message.Address{}, false,
	},
	{
		"a name holding a NUL and broken UTF-8", mail.Address{Name: "\x00Mar\xffia Perez\x00", Address: "m@example.com"},
		message.Address{Name: "Mar" + replacement + "ia Perez", Email: "m@example.com"}, true,
	},
}

// firstMailbox is the first good mailbox of an address field under test.
var firstMailbox = message.Address{Email: "first@example.com"}

// secondMailbox is the second good mailbox of an address field under test.
var secondMailbox = message.Address{Email: "second@example.com"}

// parseAddressesCases lists address field values and the mailboxes ParseAddresses answers for each.
var parseAddressesCases = []struct {
	name  string
	value string
	want  []message.Address
}{
	{"no field", "", nil},
	{"one mailbox", "Maria Perez <Maria.Perez@Example.com>", []message.Address{outsideSender}},
	{"two mailboxes", "first@example.com, second@example.com", []message.Address{firstMailbox, secondMailbox}},
	{
		"two mailboxes with no comma", "first@example.com second@example.com",
		[]message.Address{firstMailbox, secondMailbox},
	},
	{"an encoded name", "=?utf-8?q?Maria_Perez?= <maria.perez@example.com>", []message.Address{outsideSender}},
	{"empty brackets", "<>", nil},
	{
		"a raw byte in a name", "Maria Perez \xa9 Lamps <maria.perez@example.com>",
		[]message.Address{{Name: "Maria Perez " + replacement + " Lamps", Email: "maria.perez@example.com"}},
	},
	{
		"a raw byte in a quoted name", `"Maria Perez ` + "\xa9" + `" <maria.perez@example.com>`,
		[]message.Address{{Name: "Maria Perez " + replacement, Email: "maria.perez@example.com"}},
	},
	{
		"a raw byte in a name beside a good mailbox", "Maria Perez \xa9 Lamps <maria.perez@example.com>, first@example.com",
		[]message.Address{{Name: "Maria Perez " + replacement + " Lamps", Email: "maria.perez@example.com"}, firstMailbox},
	},
	{
		"junk between good mailboxes", "first@example.com, ??? junk, second@example.com",
		[]message.Address{firstMailbox, secondMailbox},
	},
	{
		"a semicolon between good mailboxes", "first@example.com; ??? junk; second@example.com",
		[]message.Address{firstMailbox, secondMailbox},
	},
	{
		"a bracket that never closes", "first@example.com, ??? <second@example.com",
		[]message.Address{firstMailbox, {Name: "???", Email: "second@example.com"}},
	},
	{"junk in the brackets", "first@example.com, ??? <no address here>", []message.Address{firstMailbox}},
	{
		"an address in a quoted name beside junk", `"note, other@example.com, note" <first@example.com>, ??? junk`,
		[]message.Address{{Name: "note, other@example.com, note", Email: "first@example.com"}},
	},
	{
		"an address in a quoted name with a raw byte", "\"note \xa9, other@example.com, note\" <first@example.com>",
		[]message.Address{{Name: "note " + replacement + ", other@example.com, note", Email: "first@example.com"}},
	},
	{
		"an address in a comment beside junk", "(note, other@example.com, note) <first@example.com>, ??? junk",
		[]message.Address{{Name: "(note, other@example.com, note)", Email: "first@example.com"}},
	},
	{
		"a bracket in a quoted name with a raw byte", "\"<other@example.com> \xa9\" <first@example.com>",
		[]message.Address{{Name: "<other@example.com> " + replacement, Email: "first@example.com"}},
	},
	{"a bracket in a comment with a raw byte", "(<other@example.com> \xa9) ???", nil},
	{
		"an escaped quote in a quoted name", `"note\", other@example.com" <first@example.com>, ??? junk`,
		[]message.Address{{Name: `note", other@example.com`, Email: "first@example.com"}},
	},
	{"a separator inside brackets", "<first@example.com, second@example.com>, ??? junk", nil},
	{"a separator in a bracket that never closes", "<first@example.com, second@example.com", nil},
	{"a comment that never closes", "(first@example.com, second@example.com", nil},
	{"a separator in a domain literal", "first@[note,other@example.com,note]", nil},
	{"a bracket in a domain literal beside junk", "first@[<other@example.com>], ??? junk", nil},
	{
		"a domain literal", "first@[192.0.2.1], second@example.com",
		[]message.Address{{Email: "first@[192.0.2.1]"}, secondMailbox},
	},
	{
		"an address as an unquoted name", "other@example.com <first@example.com>",
		[]message.Address{{Name: "other@example.com", Email: "first@example.com"}},
	},
	{
		"an address and a word as an unquoted name", "other@example.com Maria <first@example.com>",
		[]message.Address{{Name: "other@example.com Maria", Email: "first@example.com"}},
	},
	{
		"an address as an unquoted name beside junk", "other@example.com <first@example.com>, ??? junk",
		[]message.Address{{Name: "other@example.com", Email: "first@example.com"}},
	},
	{
		"a quoted left side as an unquoted name", `"Maria Perez"@example.com <first@example.com>`,
		[]message.Address{{Name: `Maria Perez"@example.com`, Email: "first@example.com"}},
	},
	{"a closing bracket in a quoted left side", `<"a>b,other@example.com,c"@example.com>, ??? junk`, nil},
	{"a closing bracket in a comment in brackets", "<(a>b,other@example.com,c)first@example.com>, ??? junk", nil},
	{"blanks inside brackets beside an address", "first@example.com <other second@example.com>", nil},
	{"blanks inside a domain beside an address", "first@example.com <other@exam ple.com>", nil},
	{
		"blanks around the address in a bracket", "??? < second@example.com >",
		[]message.Address{{Name: "???", Email: "second@example.com"}},
	},
	{"an encoded word as the left side", "=?utf-8?b?b3RoZXI=?=@example.com", nil},
	{"an encoded comma in the left side", "=?utf-8?q?other=40example.net=2Cx?=@example.com", nil},
	{"an encoded bracket in the left side", "=?utf-8?q?=3Cother=40example.net=3E?=@example.com", nil},
	{"an encoded word as the domain", "first@=?utf-8?q?example.com=0A?=", nil},
	{"an encoded line break opening the left side", "=?utf-8?q?=0Aother?=@example.com", nil},
	{"a quoted comma in the left side", `"other@example.net, x"@example.com`, nil},
	{"a quoted blank opening the left side", `" other"@example.com`, nil},
	{"a no-break space opening the left side", "\xc2\xa0@example.com", nil},
	{"a no-break space closing the domain", "first@example.com\xc2\xa0", nil},
	{"a word beside two mailboxes with no comma", "first@example.com second@example.com ???", nil},
	{
		"a separator in an encoded word", "=?utf-8?q?a,other@example.com,b?= <first@example.com>",
		[]message.Address{{Name: "=?utf-8?q?a,other@example.com,b?=", Email: "first@example.com"}},
	},
	{
		"a bracket in an encoded word", "=?utf-8?q?<other@example.com>?= <first@example.com>",
		[]message.Address{{Name: "=?utf-8?q?<other@example.com>?=", Email: "first@example.com"}},
	},
	{"a semicolon in an encoded word", "=?utf-8?q?a;other@example.com;b?= first@example.com", nil},
	{"an encoded word that never closes", "=?utf-8?q?a, other@example.com", nil},
	{
		"a separator after an escape opening an encoded word", "=?utf-8?q?=41,other@example.com,b?= <first@example.com>",
		[]message.Address{{Name: "=?utf-8?q?=41,other@example.com,b?=", Email: "first@example.com"}},
	},
	{
		"a question mark inside an encoded word", "=?utf-8?q?a?b,other@example.com,c?= <first@example.com>",
		[]message.Address{{Name: "=?utf-8?q?a?b,other@example.com,c?=", Email: "first@example.com"}},
	},
	{"an encoded word with no sections", "=?=, other@example.com", nil},
	{
		"a name in a charset the mail standard does not decode", "=?windows-1252?q?Price_=A3120?= <first@example.com>",
		[]message.Address{{Name: "Price £120", Email: "first@example.com"}},
	},
	{
		"a name in a charset nothing decodes", "=?x-unknown?q?Maria?= <first@example.com>",
		[]message.Address{{Name: "Maria", Email: "first@example.com"}},
	},
	{
		"only a quoted name holding an address", `"other@example.com" ???, first@example.com`,
		[]message.Address{firstMailbox},
	},
	{"a quote that never closes", `"other@example.com, second@example.com`, nil},
	{
		"a quoted name with a comma beside junk", `"Perez, Maria" <maria.perez@example.com>, ??? junk`,
		[]message.Address{{Name: "Perez, Maria", Email: "maria.perez@example.com"}},
	},
	{
		"an encoded name beside junk", "=?utf-8?q?Maria_Perez?= <maria.perez@example.com>, ??? junk",
		[]message.Address{outsideSender},
	},
	{"a word with no at sign", "first@example.com, junk ???", []message.Address{firstMailbox}},
	{"nothing but junk", "??? junk", nil},
	{
		"an encoded NUL beside a good mailbox", "=?utf-8?q?a=00b?=@example.com, first@example.com",
		[]message.Address{firstMailbox},
	},
	{
		"an address over the limit beside junk", addressOfBytes(255) + ", ??? junk, first@example.com",
		[]message.Address{firstMailbox},
	},
}

func TestNormalizeAddress(t *testing.T) {
	t.Parallel()

	for _, c := range normalizeAddressCases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()

			got, ok := message.NormalizeAddress(c.input)

			if got != c.want || ok != c.ok {
				t.Errorf("NormalizeAddress(%+v) = %+v, %v, want %+v, %v", c.input, got, ok, c.want, c.ok)
			}
		})
	}
}

func TestNormalizeAddressReadsAParsedHeader(t *testing.T) {
	t.Parallel()

	parsed, err := mail.ParseAddress("Maria Perez <Maria.Perez@Example.COM>")
	if err != nil {
		t.Fatalf("mail.ParseAddress() error = %v, want nil", err)
	}

	got, ok := message.NormalizeAddress(*parsed)

	if want := (message.Address{Name: "Maria Perez", Email: "maria.perez@example.com"}); got != want || !ok {
		t.Errorf("NormalizeAddress(%+v) = %+v, %v, want %+v, true", *parsed, got, ok, want)
	}
}

func TestNormalizedEmailMatchesTheContactForm(t *testing.T) {
	t.Parallel()

	for _, c := range normalizeAddressCases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()

			got, ok := message.NormalizeAddress(c.input)

			if contactForm := strings.ToLower(strings.TrimSpace(c.input.Address)); ok && got.Email != contactForm {
				t.Errorf("NormalizeAddress(%+v).Email = %q, want the contact form %q", c.input, got.Email, contactForm)
			}
		})
	}
}

func TestParseAddresses(t *testing.T) {
	t.Parallel()

	for _, c := range parseAddressesCases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()

			if got := message.ParseAddresses(c.value); !slices.Equal(got, c.want) {
				t.Errorf("ParseAddresses(%q) = %+v, want %+v", c.value, got, c.want)
			}
		})
	}
}

func TestWithoutOwn(t *testing.T) {
	t.Parallel()

	outsideAddress := message.Address{Name: "Maria Perez", Email: "maria.perez@example.com"}
	ownAddress := message.Address{Email: "info@example.com"}
	cases := []struct {
		name      string
		addresses []message.Address
		own       []string
		want      []message.Address
	}{
		{
			"the mailbox and its alias left out",
			[]message.Address{{Email: "Info@Example.com"}, outsideAddress, {Email: "sales@example.com"}},
			[]string{"info@example.com", "sales@example.com"},
			[]message.Address{outsideAddress},
		},
		{
			"an empty own list", []message.Address{ownAddress, outsideAddress}, nil,
			[]message.Address{ownAddress, outsideAddress},
		},
		{
			"the same address in To and Cc",
			slices.Concat([]message.Address{outsideAddress}, []message.Address{outsideAddress, ownAddress}), nil,
			[]message.Address{outsideAddress, ownAddress},
		},
		{
			"own addresses with blanks and capitals", []message.Address{ownAddress, outsideAddress},
			[]string{" INFO@Example.com\t"}, []message.Address{outsideAddress},
		},
		{"no addresses", nil, []string{"info@example.com"}, nil},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()

			if got := message.WithoutOwn(c.addresses, c.own); !slices.Equal(got, c.want) {
				t.Errorf("WithoutOwn(%+v, %q) = %+v, want %+v", c.addresses, c.own, got, c.want)
			}
		})
	}
}

// addressOfBytes answers an address of size bytes.
func addressOfBytes(size int) string {
	const domain = "@example.com"
	return strings.Repeat("a", size-len(domain)) + domain
}
