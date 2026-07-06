// SPDX-License-Identifier: AGPL-3.0-or-later

package main

import (
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/gopherium/alphone/internal/contact"
	"github.com/gopherium/alphone/internal/plugin"
	"github.com/gopherium/alphone/internal/postgres"
	"github.com/gopherium/alphone/plugins/whatsapp"
)

// registerPlugins wires every compiled-in plugin with its dependencies.
func registerPlugins(pool *pgxpool.Pool, getenv func(string) string) []plugin.Plugin {
	resolver := contact.NewResolver(postgres.NewContactStore(pool))
	return []plugin.Plugin{
		whatsapp.New(pool, resolver, whatsapp.Config{
			VerifyToken: getenv("ALPHONE_WHATSAPP_VERIFY_TOKEN"),
			AppSecret:   getenv("ALPHONE_WHATSAPP_APP_SECRET"),
		}),
	}
}
