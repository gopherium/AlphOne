---
title: Commands
description: Every command the alphone binary takes, what it changes, and how it answers.
---

The `alphone` binary runs the server and every task you do by hand, such
as creating an admin or minting an API token. In the container image it
lives at `/alphone`. On the [Install](/self-hosting/install/) setup, put
`docker compose exec alphone /alphone` in front of each command below,
for example `docker compose exec alphone /alphone account:list`. Add
`-T` after `exec` when you pipe input into the command, as in
[Install](/self-hosting/install/#4-start-it-and-create-the-admin-login).

Every command reads the variables listed in
[Configuration](/self-hosting/configuration/). All of them except `list`,
`help`, `version` and a `seed` preview need `ALPHONE_DATABASE_URL`.

## Listing the commands

Run with no command, the binary lists every command and exits:

```sh
alphone
```

```text
AlphOne Version %VERSION%

Usage:
  alphone <command> [flags] [arguments]

Every command answers -h. A command that offers -json answers one JSON document. A command that offers -yes is a dry run until -yes.

Available commands:
  check                 check every setting, every plugin and every command name
  help                  print the help of one command
  list                  list every command
  migrate               apply every schema step
  seed                  store the demo data
  serve                 run the server
  version               print the version
 account
  account:create-admin  create an account under a role
  account:disable       disable one account
  account:enable        enable one disabled account
  account:grant-role    give a role to every account holding none
  account:list          list every account with its role
  account:records       list who applied which change, the newest first
  account:role          set one account's role
 token
  token:create          mint a token for one account and show its secret once
  token:list            list the tokens of one account or of every account
  token:revoke          revoke one token of one account

Every command is described at https://docs.alph.one/self-hosting/commands/
```

`alphone list` prints the same. `alphone help` followed by a command, or
the command followed by `-h`, prints the help page of that command:

```sh
alphone help token:revoke
```

```text
revoke one token of one account

Usage:
  alphone token:revoke [flags]

Flags:
  -email address
    	address of the account that owns the tokens
  -id id
    	id of the token to revoke
  -yes
    	apply the change, a dry run without it
```

A plugin can offer commands of its own, named after the plugin, and they
show in the listing under the plugin's name. A plugin that fails to load
shows under `Not loaded:` after the commands, with the reason:

```text
Not loaded:
  plugin fields: ALPHONE_FIELDS_ENTRIES_MAX: must stand above zero, got "0"
```

## Previews and exit codes

`seed`, `account:grant-role`, `account:role`, `account:disable`,
`account:enable` and `token:revoke` only show what they would change
until you add `-yes`. Without it they change nothing, and they add one
line on the error stream:

```text
would set maria@example.com to admin
alphone: dry run, nothing changed, pass -yes to apply
```

Every command exits with one of three codes:

| Code | Meaning |
| --- | --- |
| `0` | The command ran, or showed what it would change. |
| `1` | The command ran and failed, for example over a refused setting, an unknown account or a refused change. The reason follows `alphone:` on the error stream. |
| `2` | The line itself is wrong, such as an unknown command, a missing flag or argument, or an unknown role. The reason follows `alphone:`, then the help page of the command when there is one. A missing `-name` on `token:create` or `-id` on `token:revoke` exits 1 instead. |

```sh
alphone frobnicate
```

```text
alphone: unknown command "frobnicate", run "alphone list" to see every command
```

## Server, schema and setup

### serve

Runs the server. It applies every migration first, the same ones
`migrate` applies, then starts the plugins and listens on
`ALPHONE_ADDR`. The container image runs `serve` when no command is
named. If your compose file or your script names a command of its own,
that command must be `serve`.

`serve` stops on `SIGTERM` or Ctrl+C. It first lets running requests
and the plugins finish, within the three shutdown graces
`ALPHONE_SHUTDOWN_GRACE`, `ALPHONE_SHUTDOWN_CANCEL_GRACE` and
`ALPHONE_SHUTDOWN_STOP_GRACE`. Whatever stops it must wait longer than
the three added together.

### migrate

Applies every schema step and exits without serving: the accounts, the
command records, the core and every plugin.

```sh
alphone migrate
```

```text
migrated accounts
migrated records
migrated core
migrated plugins
```

Running it again changes nothing. Every migration waits for one database
lock, so a starting server and `migrate` never migrate together.

### check

Reads every setting and loads every plugin without connecting to the
database:

```sh
alphone check
```

```text
settings, plugins and command names are valid
```

A refused value exits 1 and names the variable:

```sh
ALPHONE_TOKEN_TTL_DAYS=soon alphone check
```

```text
alphone: ALPHONE_TOKEN_TTL_DAYS: must be a whole number, got "soon"
```

Run it after you change the environment and before you restart the
server.

### seed

Stores the demo data: demo accounts, contacts, tasks and WhatsApp
conversations. Never run it against a production database, because the
demo logins share the public password `password1234`. Without `-yes` it
only says what it would do:

```text
would store the demo data
alphone: dry run, nothing changed, pass -yes to apply
```

With `-yes` it applies every migration, stores the core demo data, then
the demo data of each plugin, and starts no plugin:

```sh
alphone seed -yes
```

```text
migrated accounts
migrated records
migrated core
migrated plugins
seeded the core demo data
login: admin@example.com / password1234 (admin)
login: maria@example.com / password1234 (member)
alphone: demo data is for development only, never seed a production database
```

The `migrated` lines and the last warning go to the error stream.
Running it again repairs a half seeded database without duplicating
anything, and an account that already exists keeps its password. The
contacts a plugin's demo data creates raise `contact.created`, so a
webhook subscribed to that event receives them at the next start of the
server.

### version

```sh
alphone version
```

```text
alphone %VERSION%
```

With `-json` it answers one document:

```json
{
  "name": "alphone",
  "version": "%VERSION%"
}
```

## Accounts

The account commands work on the whole deployment, on purpose. They find
an account by its address whatever workspace it works in. Anyone who can
run the binary next to the database already holds every workspace, so
the commands do not narrow that.

### account:create-admin

Creates one account under a role, ready to log in. It needs `-email`,
`-name` and `-role`, and reads the password, at least 12 characters,
from the first line of its input. Typed at the `Password:` prompt or
piped in, the result is the same:

```sh
printf '%s\n' "$ADMIN_PASSWORD" | alphone account:create-admin \
  -email you@example.com -name "Your Name" -role admin
```

```text
migrated accounts
migrated records
migrated core
Password: created user you@example.com
```

It first applies the account, record and core migrations, so it works on
an empty database. It takes no `-as` and is not recorded, which makes it
the way to create the first admin, and the way back in when no account
can act. An unknown role exits 2 and names the roles, as in
`alphone: unknown role "owner", want admin or member`. An address that
is already taken exits 1 with `alphone: gouncer: email already taken`.

### account:list

Lists every account with its role, a dash when it holds none, and
whether it is enabled:

```sh
alphone account:list
```

```text
admin@example.com     admin   enabled
invited@example.com   member  enabled
disabled@example.com  member  disabled
maria@example.com     member  enabled
you@example.com       admin   enabled
```

With `-json` it answers `{"accounts": [...]}`, each account with `id`,
`email`, `name`, `role` and `disabled`.

### Changing an account

Four commands change accounts that already exist. Each one only shows
what it would change until you add `-yes`, and each one acts as the
account you name with `-as`:

| Command | What it changes |
| --- | --- |
| `account:role <email> <role> -as <address>` | Sets the role of one account. |
| `account:disable <email> -as <address>` | Disables one account. |
| `account:enable <email> -as <address>` | Enables one disabled account. |
| `account:grant-role -role <role> -as <address>` | Gives the role to every account holding none, and says how many it changed. With `-yes`, once the acting account passes its checks, it applies the account, record and core migrations before it gives the role. |

```sh
alphone account:role maria@example.com admin -as you@example.com
alphone account:role maria@example.com admin -as you@example.com -yes
```

```text
would set maria@example.com to admin
alphone: dry run, nothing changed, pass -yes to apply
set maria@example.com to admin
```

Without `-as` the command exits 2 with
`alphone: account:role wants -as <email>`. The acting account must
exist, be enabled, have been activated and hold a role that carries
`manage_users`, which in AlphOne is the admin role. Otherwise the
command exits 1 and changes nothing:

| Error | Why |
| --- | --- |
| `no account answers to nobody@example.com` | No account has that address. |
| `the account disabled@example.com is disabled` | The acting account is disabled. |
| `the account invited@example.com was never activated` | The acting account never set its password from its invitation. |
| `the account maria@example.com holds the role member, which lacks manage_users` | Its role may not manage users. |
| `the account admin@example.com holds no role, so it lacks manage_users` | It holds no role at all. |
| `the account you@example.com cannot change its own role` | No account changes its own role. |
| `the account you@example.com cannot disable itself` | No account disables itself. |

The acting account also reaches only as far as its own role. A change
that gives a role carrying a capability the acting account's role lacks
is refused, and so is a change to an account whose role carries one,
with a line such as
`the role <role> carries <capability>, which the account <address> lacks`.
With the roles AlphOne ships, an admin reaches every account. A role a
plugin declares can carry more than admin does.

The last enabled admin always stays. When two changes race, the one that
would leave no enabled admin is refused with
`<address> is the last enabled privileged account`.

These four commands need the command records. On a database that does
not hold them yet, such as one last migrated by a release from before
the command line was rebuilt, they stop with
`the command records are missing, run migrate first`, previews
included. Run `migrate`, or start `serve` once. When nobody
holds a role at all, [Updates and backups](/self-hosting/updates-and-backups/#rolling-back)
shows the way back in.

### account:records

Each change those four commands apply with `-yes` is recorded with its
time, the acting address, the command, its arguments and its flags.
Previews and refused attempts are not recorded. `account:records` lists
the newest first, 50 of them unless `-limit` or
`ALPHONE_COMMAND_RECORDS_LIMIT` asks for another number:

```sh
alphone account:records -limit 3
```

```text
2026-10-04T15:24:55Z  you@example.com  account:enable   maria@example.com
2026-10-04T15:24:55Z  you@example.com  account:disable  maria@example.com
2026-10-04T15:24:55Z  you@example.com  account:role     maria@example.com admin
```

With `-json` it answers `{"records": [...]}`, each record with
`applied_at`, `actor`, `account_id`, `command`, `args` and `flags`.

A record is stored after its change. If storing it fails, for example
because it takes longer than `ALPHONE_COMMAND_RECORD_TIMEOUT`, the
command exits 1 with the change already made.

## API tokens

`token:create`, `token:list` and `token:revoke` manage the tokens
programs use to call the API, see
[GraphQL API](/reference/graphql-api/#authenticating). Each one acts on
the account `-email` names, found by its address in upper or lower
case, and in the workspace that account works in. They take no `-as`
and are not recorded.

They apply no migration, so on a database no version has migrated yet
they exit 1. After an update they still run, so run `migrate`, or start
`serve` once, before you use them. Until then `token:list` can miss a
token the update moves into its owner's workspace.

### token:create

Mints a token for one account and shows its secret once. It needs
`-email` and `-name`:

```sh
alphone token:create -email you@example.com -name "my agent" \
  -scope tasks:read -scope contacts:read
```

```text
created token 01a10784-b944-72a3-b676-0f61036eb956
secret: a1_...
store it now, it is never shown again
scopes contacts:read tasks:read, expires 2027-01-02
```

- `-scope` grants one area and whether the token may write there, such
  as `tasks:read` or `tasks:write`. Repeat it for each area. Without it
  the token holds every area, shown as `scopes *`.
  `alphone help token:create` lists the areas.
- `-ttl` sets how many days the token lasts, or `never`. Without it the
  token lasts `ALPHONE_TOKEN_TTL_DAYS` days, ninety by default.

Without `-name` it exits 1 with `alphone: apitoken: empty name`.

### token:list

With `-email` it lists the tokens of one account, their secrets left
out:

```sh
alphone token:list -email you@example.com
```

```text
01a10784-b944-72a3-b676-0f61036eb956  my agent  scopes contacts:read tasks:read  created 2026-10-04  last used never  expires 2027-01-02
```

With `-all` it lists every token of every account in every workspace.
Each line starts with the owner's address and, after `tenant`, the id of
the workspace the token is stored in. A token whose account no longer
exists shows `(no account)` as its owner:

```sh
alphone token:list -all
```

```text
admin@example.com  tenant 00000000-0000-7000-8000-000000000001  01a10784-b957-7537-b4af-7d04cc2ce486  n8n  scopes *  created 2026-10-04  last used never  expires never
you@example.com  tenant 00000000-0000-7000-8000-000000000001  01a10784-b944-72a3-b676-0f61036eb956  my agent  scopes contacts:read tasks:read  created 2026-10-04  last used never  expires 2027-01-02
```

Pass one of `-email` and `-all`. Both or neither exit 2. With `-json`
it answers `{"tokens": [...]}`, each token with `id`, `name`, `scopes`,
`created_at`, `last_used_at` and `expires_at`, the last two `null` when
unset. With `-all` each token also carries `owner`, `null` when no
account answers for it, and `tenant_id`.

When an account moves to another workspace, the tokens it held stay in
the old one and still work. `token:list -all` shows them, but neither
`token:revoke` nor the API tokens tab of the Users page can revoke them
yet.

### token:revoke

Revokes one token of one account. It needs `-email` and `-id`, and only
names the token until you add `-yes`:

```sh
alphone token:revoke -email admin@example.com -id 01a10784-b957-7537-b4af-7d04cc2ce486
```

```text
would revoke token 01a10784-b957-7537-b4af-7d04cc2ce486 (n8n) of admin@example.com
alphone: dry run, nothing changed, pass -yes to apply
```

With `-yes` it answers `revoked token` and the id. An id the account
does not hold exits 1 with `alphone: apitoken: not found`, with or
without `-yes`. Without `-id` it exits 1 with
`alphone: parse token id: invalid UUID length: 0`.

## Old names

Releases before the command line was rebuilt took other names. The old
`token create` and `token list` spellings still work for now, and print
a notice first, such as
`alphone: "token list" is deprecated, use "token:list"`.

| Before | Now |
| --- | --- |
| `alphone` alone started the server | `alphone serve`. A run with no command lists the commands. |
| `alphone createadmin` | `alphone account:create-admin`, which also needs `-role`. The old name exits 2. |
| `alphone grantrole` | `alphone account:grant-role`, which also needs `-as` and `-yes`. The old name exits 2. |
| `alphone seed` | `alphone seed -yes`. Without `-yes` it only previews. |
| `alphone token create` | `alphone token:create`. The old spelling still works. |
| `alphone token list` | `alphone token:list`. The old spelling still works. |
| `alphone token revoke` | `alphone token:revoke`, which also needs `-yes`. The old spelling exits 2, so a preview is never mistaken for a revoke. |
