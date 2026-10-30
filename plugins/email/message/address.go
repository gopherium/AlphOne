// SPDX-License-Identifier: Elastic-2.0

package message

import (
	"net/mail"
	"strings"
)

// Address is one mailbox a message names, its email in the form a contact stores.
type Address struct {
	Name  string
	Email string
}

// NormalizeAddress answers a with its name trimmed and its email trimmed and lowercased, false for an empty email.
func NormalizeAddress(a mail.Address) (Address, bool) {
	email := normalEmail(a.Address)
	if email == "" {
		return Address{}, false
	}
	return Address{Name: strings.TrimSpace(a.Name), Email: email}, true
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
