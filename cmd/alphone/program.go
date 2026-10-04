// SPDX-License-Identifier: Elastic-2.0

package main

import (
	"context"

	"github.com/gopherium/framework/gonsole"
	accounts "github.com/gopherium/framework/gonsole/auth"

	"github.com/gopherium/alphone/internal/role"
	"github.com/gopherium/alphone/internal/version"
	"github.com/gopherium/alphone/sdk"
)

// program returns the alphone command line over getenv and the compiled plugins.
func program(getenv func(string) string, plugins func(sdk.Deps) ([]sdk.Plugin, error)) gonsole.Program {
	return programOver(role.Default, getenv, plugins)
}

// programOver returns the alphone command line declaring plugin roles into registry, serve into role.Default.
func programOver(
	registry *role.Registry, getenv func(string) string, plugins func(sdk.Deps) ([]sdk.Plugin, error),
) gonsole.Program {
	return gonsole.Program{
		Name:       "alphone",
		Title:      "AlphOne",
		Version:    version.Version(),
		Footer:     "Every command is described at https://docs.alph.one/self-hosting/commands/",
		Env:        settingsEnv(getenv),
		Database:   "DATABASE_URL",
		Serve:      serve(plugins),
		Migrations: migrations(),
		Seed:       seedCore,
		Commands:   accounts.Commands(accountConfig(registry)),
		Plugins:    loadPlugins(registry, plugins),
	}
}

// serve returns the command that serves the API and the web application over the compiled plugins.
func serve(plugins func(sdk.Deps) ([]sdk.Plugin, error)) func(context.Context, gonsole.Call) error {
	return func(ctx context.Context, call gonsole.Call) error {
		return run(ctx, call.Env.Getenv, call.Stderr, plugins)
	}
}

// seedCore stores the demo data in the database the call's settings name.
func seedCore(ctx context.Context, call gonsole.Call) error {
	return seed(ctx, call.Env.Getenv, call.Stdout)
}
