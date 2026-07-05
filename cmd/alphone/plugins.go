// SPDX-License-Identifier: AGPL-3.0-or-later

package main

import (
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/gopherium/alphone/internal/plugin"
	"github.com/gopherium/alphone/plugins/whatsapp"
)

// registerPlugins wires every compiled-in plugin with its dependencies.
func registerPlugins(pool *pgxpool.Pool) []plugin.Plugin {
	return []plugin.Plugin{
		whatsapp.New(pool),
	}
}
