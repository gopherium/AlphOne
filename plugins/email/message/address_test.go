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
		"a name holding a NUL and broken UTF-8", mail.Address{Name: "\x00Mar\xffia Perez\x00", Address: "m@example.com"},
		message.Address{Name: "Mar" + replacement + "ia Perez", Email: "m@example.com"}, true,
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

	first := message.Address{Email: "first@example.com"}
	second := message.Address{Email: "second@example.com"}
	cases := []struct {
		name  string
		value string
		want  []message.Address
	}{
		{"no field", "", nil},
		{"one mailbox", "Maria Perez <Maria.Perez@Example.com>", []message.Address{outsideSender}},
		{"two mailboxes", "first@example.com, second@example.com", []message.Address{first, second}},
		{"two mailboxes with no comma", "first@example.com second@example.com", []message.Address{first, second}},
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
			[]message.Address{{Name: "Maria Perez " + replacement + " Lamps", Email: "maria.perez@example.com"}, first},
		},
		{
			"junk between good mailboxes", "first@example.com, ??? junk, second@example.com",
			[]message.Address{first, second},
		},
		{
			"a semicolon between good mailboxes", "first@example.com; ??? junk; second@example.com",
			[]message.Address{first, second},
		},
		{
			"a bracket that never closes", "first@example.com, ??? <second@example.com",
			[]message.Address{first, {Name: "???", Email: "second@example.com"}},
		},
		{"junk in the brackets", "first@example.com, ??? <no address here>", []message.Address{first}},
		{"a word with no at sign", "first@example.com, junk ???", []message.Address{first}},
		{"nothing but junk", "??? junk", nil},
		{
			"an encoded NUL beside a good mailbox", "=?utf-8?q?a=00b?=@example.com, first@example.com",
			[]message.Address{first},
		},
		{
			"an address over the limit beside junk", addressOfBytes(255) + ", ??? junk, first@example.com",
			[]message.Address{first},
		},
	}

	for _, c := range cases {
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
