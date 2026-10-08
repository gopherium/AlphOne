---
title: Commands and settings
description: Offer a command from your plugin, and read your plugin's settings from the environment.
---

Your plugin can add commands to the `alphone` binary, and read settings
of its own from the environment. Both come through the SDK, so the
plugin imports nothing else from AlphOne. The binary is built on the
Gopherium command line base, and its
[plugin commands page](https://docs.gopherium.org/command-line/plugin-commands/)
covers every field in full.

This page uses one plugin, `archive`, that deletes archived rows once
they are older than a keep window.

## Read your settings

`deps.Env` reads the settings under the `ALPHONE_` prefix, each value
trimmed of surrounding spaces. Read them in `Register`:

```go
// Register builds the archive plugin, refusing a malformed setting and connecting nowhere yet.
func Register(deps sdk.Deps) (*Plugin, error) {
	keep, err := deps.Env.Duration("ARCHIVE_KEEP", 90*24*time.Hour)
	if err != nil {
		return nil, err
	}
	pool, err := pgxpool.New(context.Background(), deps.DatabaseURL)
	if err != nil {
		return nil, fmt.Errorf("archive: connect database: %w", err)
	}
	return &Plugin{pool: pool, keep: keep}, nil
}
```

`deps.Env.Duration("ARCHIVE_KEEP", ...)` reads `ALPHONE_ARCHIVE_KEEP`.
These are the readers:

| Reader | Answers |
| --- | --- |
| `Value(name)` | The value, empty when the setting is unset. |
| `Required(name)` | The value, or an error such as `ALPHONE_ARCHIVE_TOKEN is required` when it is empty. |
| `Count(name, fallback, bounds...)` | A whole number above zero, the fallback when the setting is unset. |
| `Duration(name, fallback, bounds...)` | A duration above zero such as `30s`, the fallback when the setting is unset. |
| `Within(more)` | The settings under a longer prefix. `deps.Env.Within("ARCHIVE_").Count("BATCH", 500)` reads `ALPHONE_ARCHIVE_BATCH`. |

Two bounds narrow `Count` and `Duration`. `sdk.AtMost(10000)` refuses a
value above 10000, and `sdk.AllowZero()` accepts zero too. For any
other shape, `sdk.Parse` reads the value through a function of your own
and puts the setting's name in front of its error:

```go
color, err := sdk.Parse(deps.Env, "ARCHIVE_COLOR", "blue", parseColor)
```

Every error names the setting and what it must be, such as
`ALPHONE_ARCHIVE_KEEP: must be a duration like 30s, got "soon"`. Return
it from `Register` and your plugin does not load. `alphone list` shows
it under `Not loaded:`, `alphone check` names it, and `serve`,
`migrate`, `seed -yes`, `account:create-admin` and the account commands
that take `-as` stop with it. Add each setting to
`.env.example` and to [Configuration](/self-hosting/configuration/).

## Offer a command

A plugin offers commands by implementing `sdk.CommandProvider`, whose
one method returns them as `[]sdk.Command`. Every command name starts
with the plugin's id and a colon:

```go
var _ sdk.CommandProvider = (*Plugin)(nil)

// Commands returns the commands the archive plugin offers.
func (p *Plugin) Commands() []sdk.Command {
	return []sdk.Command{{
		Name:    "archive:purge",
		Summary: "delete the archived rows older than the keep window",
		Writes:  true,
		Run:     p.purge,
	}}
}

// purge deletes the rows past the keep window, only counting them until the call applies.
func (p *Plugin) purge(ctx context.Context, call sdk.Call) error {
	cutoff := time.Now().Add(-p.keep)
	if !call.Apply {
		var rows int
		err := p.pool.QueryRow(ctx,
			"SELECT count(*) FROM plugin_archive.rows WHERE archived_at < $1", cutoff).Scan(&rows)
		if err != nil {
			return err
		}
		_, err = fmt.Fprintf(call.Stdout, "would delete %d rows\n", rows)
		return err
	}
	deleted, err := p.pool.Exec(ctx, "DELETE FROM plugin_archive.rows WHERE archived_at < $1", cutoff)
	if err != nil {
		return err
	}
	_, err = fmt.Fprintf(call.Stdout, "deleted %d rows\n", deleted.RowsAffected())
	return err
}
```

The `var _` line turns a missing or misspelled `Commands` method into a
build error. Without it the binary skips the plugin's commands and says
nothing.

`archive:purge` then shows in `alphone list` under the plugin's id.
`Writes: true` makes it a preview until `-yes`, as described in
[Commands](/self-hosting/commands/#previews-and-exit-codes).
`call.Apply` is true once the line holds `-yes`, and without it the
binary adds `alphone: dry run, nothing changed, pass -yes to apply`
after your output. Wrap an error in `sdk.Misuse` when the line itself
is wrong, such as a flag value the command cannot use, so the command
exits 2 and prints its help page. Any other error exits 1.

A plugin id may not be the name of a base command or of a core
namespace: `help`, `list`, `version`, `check`, `serve`, `migrate`,
`seed`, `account` and `token`. `make generate` refuses such an id.

## Flags a run must set

`Flags` declares the command's own flags with Go's standard `flag`
package, and `Needs` names the ones every run must set. This second
command brings back the rows archived since the day the line names:

```go
// restore returns archive:restore, which brings back the rows archived since the day -since names.
func (p *Plugin) restore() sdk.Command {
	return sdk.Command{
		Name:    "archive:restore",
		Summary: "bring back the rows archived since one day",
		Flags: func(fs *flag.FlagSet) {
			fs.String("since", "", "first `day` to bring back, such as 2026-10-01")
		},
		Needs:  []string{"since"},
		Writes: true,
		Run: func(ctx context.Context, call sdk.Call) error {
			day := call.Flags["since"]
			since, err := time.Parse(time.DateOnly, day)
			if err != nil {
				return sdk.Misuse(fmt.Errorf("archive:restore: -since %q is not a day like 2026-10-01", day))
			}
			return p.bringBack(ctx, call, since)
		},
	}
}
```

Add `p.restore()` to the list `Commands` returns. A line that leaves
`-since` out, or gives it empty text or only spaces, exits 2 with
`alphone: archive:restore wants -since <day>` and the help page. The
binary checks this as soon as it has read the line, before it calls
`Run`, so `Run` never has to look for a missing `-since` itself.
`alphone archive:restore -h` still prints the help page.

The placeholder `day` is the word in backquotes in the flag's usage.
Without backquotes it names the kind of value, such as `string` or
`int`, or just `value`.

The binary checks the text the line types for the flag, not the value
the flag reads back. So a flag declared with `fs.Func`, which keeps no
value of its own, works in `Needs` too. A default does not count. A
flag declared as `fs.Int("batch", 500, ...)` and named in `Needs` still
stops a line that leaves `-batch` out. Each name in `Needs` must be a
flag that `Flags` declares and that takes a value, unlike a `bool`
flag. Otherwise the binary drops the command, `alphone check` names it,
and running it exits 1.

`Needs` only checks that a value is there. To stop a bad value, such as
`-since soon`, `Run` returns the error wrapped in `sdk.Misuse`, as
above, so the run exits 2 with the help page the same way.
[Writing commands](https://docs.gopherium.org/command-line/writing-commands/)
covers `Needs` in full.

## An acting account

The acting account is the account of the person who runs the command.
Set `Capability` to one of the capabilities the roles carry, such as
`manage_users`, and the command wants `-as <email>`:

```go
// Commands returns the commands the archive plugin offers.
func (p *Plugin) Commands() []sdk.Command {
	return []sdk.Command{{
		Name:       "archive:purge",
		Summary:    "delete the archived rows older than the keep window",
		Writes:     true,
		Capability: "manage_users",
		Run:        p.purge,
	}, p.restore()}
}
```

A line that leaves `-as` out, or gives it empty text or only spaces,
exits 2 with `alphone: archive:purge wants -as <email>` and the help
page. `Run` reads the address as `call.Actor`, exactly as typed, so
trim it and lower-case it before a lookup.

The `alphone` binary checks the acting account through its `Authorize`
function, on a dry run too. A command that names a `Capability` is
refused before `Run` unless the `-as` account exists, is enabled and
activated, and holds a role with that capability. The run then exits 1
and says why, such as
`the account maria@example.com holds the role member, which lacks manage_users`.
[Changing an account](/self-hosting/commands/#changing-an-account)
lists every reason.

Each run such a command applies is stored as one record, which
`alphone account:records` lists. The binary's `Record` function stores
it once `Run` has made the change. Here `you@example.com` is an admin:

```sh
alphone archive:purge -as you@example.com
alphone archive:purge -as you@example.com -yes
alphone account:records -limit 1
```

```text
would delete 12 rows
alphone: dry run, nothing changed, pass -yes to apply
deleted 12 rows
2026-10-08T09:12:40Z  you@example.com  archive:purge
```

Previews and refused attempts are not recorded. If storing the record
fails, the run exits 1 with the change already made. The record keeps
the command's arguments and flags, so take a secret, such as a
password, only on `call.Stdin`.

The roles AlphOne ships carry two capabilities, `manage_users` and
`manage_webhooks`, both held by admin. A plugin that implements
`sdk.RoleProvider` can give a role a capability of its own. Name one no
role carries, and every account is refused. AlphOne sets `Authorize`
and `Record` for every plugin, so yours writes neither.
[Writing commands](https://docs.gopherium.org/command-line/writing-commands/#an-acting-account)
shows both from the program's side.

## What the binary expects from a plugin

The binary registers your plugin for `list`, `check`, `migrate`,
`seed -yes`, `account:create-admin`, the account commands that take
`-as` and the plugin's own commands, and never starts it for any of
them. So:

- `Register` must not connect to anything or start any work. A pool
  from `pgxpool.New` is fine, because it connects on first use.
- `Stop` must work when `Start` never ran, and return by the time its
  context ends.
- A plugin that migrates owns one schema, named `plugin_` followed by
  its id, with each hyphen written as an underscore. The plugin
  `note-archive` owns `plugin_note_archive`. It migrates under goose's
  session lock, and treats the same schema created by another session as
  success, as the plugins under `plugins/` do. `go test ./cmd/alphone/`
  checks every registered plugin that migrates against both rules.
