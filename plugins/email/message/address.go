// SPDX-License-Identifier: Elastic-2.0

package message

import (
	"net/mail"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/jhillyerd/enmime/v2"
)

// emailLimit is the longest email the mail standard allows.
const emailLimit = 254

// Address is one mailbox a message names, its email in the form a contact stores.
type Address struct {
	Name  string
	Email string
}

// NormalizeAddress answers a with its name cleaned and its email trimmed and lowercased, false for an unusable email.
func NormalizeAddress(a mail.Address) (Address, bool) {
	email := normalEmail(a.Address)
	if email == "" || !usableEmail(strings.TrimSpace(a.Address)) {
		return Address{}, false
	}
	return Address{Name: strings.TrimSpace(cleanText(a.Name)), Email: email}, true
}

// ParseAddresses answers the normalized mailboxes an address field value names, skipping what it cannot read.
func ParseAddresses(value string) []Address {
	list, err := enmime.ParseAddressList(value)
	if err != nil {
		return scanAddresses(value)
	}
	var kept []Address
	for _, a := range list {
		if address, ok := NormalizeAddress(*a); ok {
			kept = append(kept, address)
		}
	}
	return kept
}

// scanAddresses answers the normalized mailboxes in the separated pieces of a value the address reader turned down.
func scanAddresses(value string) []Address {
	var kept []Address
	for piece := range strings.FieldsFuncSeq(value, endsAddress) {
		if address, ok := NormalizeAddress(pieceAddress(piece)); ok {
			kept = append(kept, address)
		}
	}
	return kept
}

// endsAddress reports whether r separates two pieces of an address field.
func endsAddress(r rune) bool {
	return r == ',' || r == ';'
}

// pieceAddress answers the mailbox one piece of a field names, bracketed or bare, the zero value when it names none.
func pieceAddress(piece string) mail.Address {
	name, bracketed, found := strings.Cut(piece, "<")
	if !found {
		name, bracketed = "", piece
	}
	email, _, _ := strings.Cut(bracketed, ">")
	parsed, err := mail.ParseAddress("<" + strings.TrimSpace(email) + ">")
	if err != nil {
		return mail.Address{}
	}
	return mail.Address{Name: strings.Trim(name, " \t\""), Address: parsed.Address}
}

// usableEmail reports whether email fits the length limit and is UTF-8 with no control character.
func usableEmail(email string) bool {
	return len(email) <= emailLimit && utf8.ValidString(email) && !strings.ContainsFunc(email, unicode.IsControl)
}

// WithoutOwn answers addresses in order and each email once, without the emails in own.
func WithoutOwn(addresses []Address, own []string) []Address {
	skip := make(map[string]bool, len(own)+len(addresses))
	for _, email := range own {
		skip[normalEmail(email)] = true
	}
	var kept []Address
	for _, a := range addresses {
		email := normalEmail(a.Email)
		if !skip[email] {
			skip[email] = true
			kept = append(kept, a)
		}
	}
	return kept
}

// normalEmail answers email trimmed and lowercased.
func normalEmail(email string) string {
	return strings.ToLower(strings.TrimSpace(email))
}
