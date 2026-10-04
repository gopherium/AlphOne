// SPDX-License-Identifier: Elastic-2.0

package main

import (
	"context"
	"slices"

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
	accountCommands := accountConfig(registry)
	return gonsole.Program{
		Name:       "alphone",
		Title:      "AlphOne",
		Version:    version.Version(),
		Footer:     "Every command is described at https://docs.alph.one/self-hosting/commands/",
		Env:        settingsEnv(getenv),
		Database:   "DATABASE_URL",
		Renamed:    map[string]string{"token create": "token:create", "token list": "token:list"},
		Serve:      serve(plugins),
		Validate:   validate(accountCommands),
		Migrations: migrations(),
		Seed:       seedCore,
		Commands:   coreCommands(accountCommands),
		Plugins:    loadPlugins(registry, plugins),
		Authorize:  accounts.Authorize(accountCommands),
		Record:     accounts.Record(accountCommands),
	}
}

// coreCommands returns the account commands over cfg, the records they keep and the token commands.
func coreCommands(cfg accounts.Config) []gonsole.Command {
	return slices.Concat(accounts.Commands(cfg), []gonsole.Command{accounts.Records(cfg)}, tokenCommands())
}

// serve returns the command that serves the API and the web application over the compiled plugins.
func serve(plugins func(sdk.Deps) ([]sdk.Plugin, error)) func(context.Context, gonsole.Call) error {
	return func(ctx context.Context, call gonsole.Call) error {
		return run(ctx, call.Env.Getenv, call.Stderr, plugins)
	}
}

// validate returns the check of every setting the server and the account hooks of cfg read, reaching no database.
func validate(cfg accounts.Config) func(context.Context, gonsole.Call) error {
	return func(_ context.Context, call gonsole.Call) error {
		if _, err := loadRunConfig(call.Env.Getenv); err != nil {
			return err
		}
		return cfg.Validate(call.Env)
	}
}

// seedCore stores the core demo data in the database the call's settings name.
func seedCore(ctx context.Context, call gonsole.Call) error {
	return seed(ctx, call.Env.Getenv, call.Stdout)
}
