// SPDX-License-Identifier: AGPL-3.0-or-later

package main

import (
	"context"

	"github.com/gopherium/alphone/internal/contact"
	"github.com/gopherium/alphone/sdk"
)

type resolverBridge struct {
	resolver *contact.Resolver
}

// Resolve resolves a channel identifier to an [sdk.Contact] via the underlying contact resolver.
func (b resolverBridge) Resolve(ctx context.Context, channel sdk.Channel, identifier, displayName string) (sdk.Contact, error) {
	owner, err := b.resolver.Resolve(ctx, contact.Channel(channel), identifier, displayName)
	if err != nil {
		return sdk.Contact{}, err
	}
	return sdk.Contact{ID: owner.ID, Name: owner.Name}, nil
}
