// SPDX-License-Identifier: AGPL-3.0-or-later

package main

import (
	"context"

	"github.com/gopherium/alphone/internal/contact"
	"github.com/gopherium/alphone/plugins/whatsapp"
	"github.com/gopherium/alphone/sdk"
)

type resolverBridge struct {
	resolver *contact.Resolver
}

func (b resolverBridge) Resolve(ctx context.Context, channel sdk.Channel, identifier, displayName string) (sdk.Contact, error) {
	owner, err := b.resolver.Resolve(ctx, contact.Channel(channel), identifier, displayName)
	if err != nil {
		return sdk.Contact{}, err
	}
	return sdk.Contact{ID: owner.ID, Name: owner.Name}, nil
}

// registerPlugins builds every compiled-in plugin from deps.
func registerPlugins(deps sdk.Deps) ([]sdk.Plugin, error) {
	whatsappPlugin, err := whatsapp.Register(deps)
	if err != nil {
		return nil, err
	}
	return []sdk.Plugin{whatsappPlugin}, nil
}
