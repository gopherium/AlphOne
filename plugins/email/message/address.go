// SPDX-License-Identifier: Elastic-2.0

package message

import (
	"io"
	"mime"
	"net/mail"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/jhillyerd/enmime/v2"
)

// emailLimit is the longest email the mail standard allows.
const emailLimit = 254

// addressReader reads addresses as the mail standard writes them, passing every charset through.
var addressReader = mail.AddressParser{WordDecoder: &mime.WordDecoder{
	CharsetReader: func(_ string, input io.Reader) (io.Reader, error) { return input, nil },
}}

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
	list, err := addressField(value).read()
	if err != nil {
		return addressField(value).mailboxes()
	}
	return normalized(list)
}

// normalized answers the mailboxes of list that read as one mailbox and NormalizeAddress keeps.
func normalized(list []*mail.Address) []Address {
	var kept []Address
	for _, a := range list {
		if address, ok := NormalizeAddress(*a); ok && oneMailbox(a) {
			kept = append(kept, address)
		}
	}
	return kept
}

// oneMailbox reports whether the email of a reads back as itself with no encoded word and no blank around it.
func oneMailbox(a *mail.Address) bool {
	read, err := mail.ParseAddress(a.Address)
	return err == nil && read.Address == a.Address && read.Address == strings.TrimSpace(a.Address) &&
		!strings.Contains(a.Address, "=?")
}

// addressField is the text of an address field, or one piece of it.
type addressField string

// read answers the mailboxes of f as addressReader reads them, with the names enmime decodes when it can.
func (f addressField) read() ([]*mail.Address, error) {
	list, err := addressReader.ParseList(string(f))
	if err != nil {
		return nil, err
	}
	if named, err := enmime.ParseAddressList(string(f)); err == nil {
		for i, a := range named {
			list[i].Name = a.Name
		}
	}
	return list, nil
}

// mailboxes answers the mailboxes in each piece of a field addressReader turned down.
func (f addressField) mailboxes() []Address {
	var kept []Address
	for rest := f; rest != ""; {
		var piece addressField
		piece, rest = rest.nextPiece()
		kept = append(kept, piece.named()...)
	}
	return kept
}

// nextPiece answers f up to its first separator outside quotes, comments and brackets, and the text after it.
func (f addressField) nextPiece() (addressField, addressField) {
	bracketed := false
	for i := 0; i < len(f); i = f.past(i) {
		switch {
		case f[i] == '<' || f[i] == '>':
			bracketed = f[i] == '<'
		case !bracketed && (f[i] == ',' || f[i] == ';'):
			return f[:i], f[i+1:]
		}
	}
	return f, ""
}

// past answers where a scan of f goes on after index i, beyond any quoted, commented, literal or encoded text there.
func (f addressField) past(i int) int {
	text := string(f[i:])
	switch {
	case text[0] == '"':
		return len(f) - len(afterQuoted(text))
	case text[0] == '(':
		return len(f) - len(afterComment(text))
	case text[0] == '[':
		_, rest, _ := strings.Cut(text, "]")
		return len(f) - len(rest)
	case strings.HasPrefix(text, "=?"):
		_, rest, _ := strings.Cut(text, "?=")
		return len(f) - len(rest)
	}
	return i + 1
}

// named answers the mailboxes the piece f names, whole, from the bracket outside its quotes, or word by word.
func (f addressField) named() []Address {
	if list, err := f.read(); err == nil {
		return normalized(list)
	}
	for i := 0; i < len(f); i = f.past(i) {
		if f[i] == '<' {
			return f.bracketed(i)
		}
	}
	return f.words()
}

// words answers a mailbox for each word of the piece f, nothing when any word is not one.
func (f addressField) words() []Address {
	var list []*mail.Address
	for word := range strings.FieldsSeq(string(f)) {
		read, err := addressReader.Parse(word)
		if err != nil {
			return nil
		}
		list = append(list, read)
	}
	return normalized(list)
}

// bracketed answers the mailbox in the bracket of the piece f opening at index open, named by the text before it.
func (f addressField) bracketed(open int) []Address {
	email, _, _ := strings.Cut(string(f[open+1:]), ">")
	parsed, err := mail.ParseAddress("<" + strings.TrimSpace(email) + ">")
	if err != nil {
		return nil
	}
	parsed.Name = strings.Trim(string(f[:open]), " \t\"")
	return normalized([]*mail.Address{parsed})
}

// usableEmail reports whether email in its stored form fits the length limit and is UTF-8 with no control character.
func usableEmail(email string) bool {
	return len(normalEmail(email)) <= emailLimit && utf8.ValidString(email) &&
		!strings.ContainsFunc(email, unicode.IsControl)
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
