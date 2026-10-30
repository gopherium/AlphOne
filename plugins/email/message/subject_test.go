// SPDX-License-Identifier: Elastic-2.0

package message_test

import (
	"testing"

	"github.com/gopherium/alphone/plugins/email/message"
)

func TestNormalizeSubject(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name    string
		subject string
		want    string
	}{
		{"a reply tag", "Re: Quote", "Quote"},
		{"an upper case reply tag", "RE: Quote", "Quote"},
		{"a lower case reply tag", "re: Quote", "Quote"},
		{"a reply tag with a bracketed count", "Re[2]: Quote", "Quote"},
		{"a reply tag with a caret count", "Re^2: Quote", "Quote"},
		{"a reply tag with a parenthesized count", "Re(2): Quote", "Quote"},
		{"a count with two digits", "Re[12]: Quote", "Quote"},
		{"a short forward tag", "Fw: Quote", "Quote"},
		{"an upper case short forward tag", "FW: Quote", "Quote"},
		{"a forward tag", "Fwd: Quote", "Quote"},
		{"an upper case forward tag", "FWD: Quote", "Quote"},
		{"a forward tag with a count", "Fwd[3]: Quote", "Quote"},
		{"stacked tags", "Re: Fwd: RE: Quote", "Quote"},
		{"no blank after the colon", "Re:Quote", "Quote"},
		{"extra blanks around the colon and the words", "  Re :  Big \t Quote  ", "Big Quote"},
		{"a word starting with the tag letters", "Request for a quote", "Request for a quote"},
		{"a longer word before a colon", "Rex: Quote", "Rex: Quote"},
		{"a tag with no colon", "Re Quote", "Re Quote"},
		{"a tag in the middle", "Quote Re: draft", "Quote Re: draft"},
		{"a tag alone", "Re: ", ""},
		{"a leading list tag", "[team] Re: Quote", "[team] Re: Quote"},
		{"empty", "", ""},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()

			if got := message.NormalizeSubject(c.subject); got != c.want {
				t.Errorf("NormalizeSubject(%q) = %q, want %q", c.subject, got, c.want)
			}
		})
	}
}
