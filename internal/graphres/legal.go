// SPDX-License-Identifier: Elastic-2.0

package graphres

import (
	"context"

	"github.com/gopherium/alphone/graph/model"
)

// LegalLinks carries the addresses of the install's legal pages.
type LegalLinks struct {
	// Privacy is the address of the privacy policy. Empty answers none.
	Privacy string
	// Terms is the address of the terms of service. Empty answers none.
	Terms string
}

// LegalLinks answers the addresses of the install's legal pages.
func (q QueryResolvers) LegalLinks(context.Context) (*model.LegalLinks, error) {
	links := q.root.Legal
	return &model.LegalLinks{PrivacyURL: legalAddress(links.Privacy), TermsURL: legalAddress(links.Terms)}, nil
}

// legalAddress answers an address, or none when it is empty.
func legalAddress(address string) *string {
	if address == "" {
		return nil
	}
	return &address
}
