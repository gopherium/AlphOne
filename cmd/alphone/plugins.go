// SPDX-License-Identifier: AGPL-3.0-or-later

package main

import (
	"context"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/gopherium/alphone/internal/contact"
	"github.com/gopherium/alphone/internal/postgres"
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

// registerPlugins wires every compiled-in plugin with its dependencies.
func registerPlugins(pool *pgxpool.Pool, getenv func(string) string) []sdk.Plugin {
	resolver := resolverBridge{resolver: contact.NewResolver(postgres.NewContactStore(pool))}
	return []sdk.Plugin{
		whatsapp.New(pool, resolver, whatsapp.Config{
			VerifyToken: getenv("ALPHONE_WHATSAPP_VERIFY_TOKEN"),
			AppSecret:   getenv("ALPHONE_WHATSAPP_APP_SECRET"),
		}),
	}
}
