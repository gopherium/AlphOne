// SPDX-License-Identifier: Elastic-2.0

package message_test

import (
	"net/mail"
	"slices"
	"strings"
	"testing"

	"github.com/gopherium/alphone/plugins/email/message"
)

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
