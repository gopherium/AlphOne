---
title: Updates and backups
description: How releases reach your server, how to update automatically or roll back, and how to back up and restore the database.
---

## How releases work

Every AlphOne release is a git tag (`vx.y.z`) that publishes a container
image to `ghcr.io/gopherium/alphone` under three tags:

- the version, e.g. `:x.y.z`
- the exact commit, e.g. `:sha-5495093`
- `:latest`, republished on every release

Migrations run automatically on container start, so updating is pulling
a newer image and recreating the container.

## Updating by hand

```sh
cd /srv/alphone
docker compose pull alphone
docker compose up -d alphone
```

Recreating the container starts `serve`, which applies the new version's
migrations before it listens. Let that happen before the first account
change after an update, or run `alphone migrate`, which applies them
without starting the server, see [Commands](/self-hosting/commands/).
After an update from a release that did not keep command records yet,
the account commands that take `-as` change nothing until the
migrations ran. They stop with
`the command records are missing, run migrate first`.

## Updating automatically

Any watcher that reacts to a republished `:latest` digest works. The
compose file in [Install](/self-hosting/install/) already carries labels
for [What's Up Docker](https://getwud.github.io/wud/) (WUD):

- `wud.watch: "true"` with `wud.watch.digest: "true"` on `alphone`
  watches the `:latest` digest and picks up each release.
- `wud.watch: "false"` on `postgres` makes sure the database is never
  auto-updated. A major PostgreSQL jump breaks its data directory.

On the WUD side, configure a ghcr registry (a public image needs no
token) and a trigger whose scope includes the `alphone` container. Start
with a notification-only trigger if you want to review updates before
they apply, then switch to auto once you trust the flow.

## Updating from an older release

Coming from one release back or older, the update changes what an
existing install sends and accepts. Read this before it reaches your
server, because a watcher on `:latest` applies it on its own.

- AlphOne refuses to deliver webhooks to internal addresses. A receiver
  on your own network, such as n8n at `http://n8n:5678` on the same
  Docker network or at `http://localhost:5678` on the same machine,
  receives nothing until you list it, for example with
  `ALPHONE_WEBHOOK_ALLOWED_HOSTS=n8n:5678`. Its subscription stays and
  looks active. The only signs are the warning
  `refusing a webhook delivery to an internal address` in the log and
  the `last_error` of its deliveries. See
  [Webhooks](/self-hosting/configuration/#webhooks).
- Webhook deliveries ignore `HTTP_PROXY` and `HTTPS_PROXY` and connect
  to the receiver directly.
- A receiver that answers with a redirect, any `3xx`, fails the
  delivery attempt. Subscribe the final address.
- Disabling an account stops its webhooks.
- A browser write sent from a page at another origin is refused with
  `request_cross_origin`. A browser from before 2023 does not send
  `Sec-Fetch-Site`, so AlphOne compares its `Origin` with the `Host`
  header. Behind a reverse proxy, the proxy has to pass the visitor's
  `Host` through unchanged, or such a browser is refused on AlphOne's
  own pages too. `X-Forwarded-Host` is not read. See
  [Cross-origin writes](/self-hosting/configuration/#cross-origin-writes).
- The command line was rebuilt. The image still starts `serve` by
  itself, but a `command:` or script of your own that ran `alphone`
  with no command now only lists the commands, so it needs `serve`.
  `createadmin` and `grantrole` are refused under their old names, see
  [Old names](/self-hosting/commands/#old-names).

## Rolling back

Pin the previous version and re-up:

```yaml
    image: ghcr.io/gopherium/alphone:x.y.z   # the version to roll back to
    labels:
      wud.watch: "false"   # pause auto-updates while pinned
```

```sh
docker compose up -d alphone
```

A release from before the command line was rebuilt serves when it runs
with no command, and refuses `serve`. Remove `serve` from any
`command:` or script you added before you pin such a release. It also
refuses every command in the Now column of
[Old names](/self-hosting/commands/#old-names), so a script that runs
one of them stops working.

One caution: rolling the app back does not roll the database back.
Migrations only move forward, so if the newer version already migrated
the schema, restore the matching backup instead of just pinning the
older image.

If you ever roll a migration back by hand, know that the role each
account holds is the thing most easily lost. It lives in a `role`
column on the account row. The migration that added that column drops
it on the way down, and every promotion and demotion goes with it.
Save the roles first:

```sh
docker compose exec -T postgres psql -U alphone alphone -v ON_ERROR_STOP=1 \
  -c "\\copy (SELECT id, role FROM auth.users) TO STDOUT WITH (FORMAT csv)" \
  > roles.csv.part && mv roles.csv.part roles.csv
```

That keeps every role exactly as stored, including any role a plugin declared,
and CSV quoting handles whatever the values contain. `ON_ERROR_STOP=1` matters
in both commands, because without it `psql` carries on after a failed statement
and leaves you an incomplete file that looks like a good one. The export writes
`roles.csv.part` and renames it only once the command succeeds, because your
shell creates the file it redirects into before `psql` even starts, so a failure
halfway through would otherwise hand you a truncated `roles.csv` in place of the
one you were counting on. Put the roles back
once the column exists again, reading the file on the machine you run the
command from:

```sh
docker compose exec -T postgres psql -U alphone alphone -v ON_ERROR_STOP=1 \
  -c 'CREATE TEMP TABLE restored (id uuid PRIMARY KEY, role text NOT NULL)' \
  -c "\\copy restored (id, role) FROM STDIN WITH (FORMAT csv)" \
  -c 'UPDATE auth.users u SET role = r.role FROM restored r WHERE r.id = u.id' \
  < roles.csv
```

Both commands stream through `psql`, so the file never has to exist inside the
container. Taking a full `pg_dump` before any rollback is simpler still, and it
is what the backup section below sets up anyway.

An account that ends up holding no role still works contacts and tasks,
because no field of the product asks for a capability. What it loses is
user management, so a rollback that strips every role can leave nobody
able to promote anyone back. `account:grant-role` gives a role to every
account holding none, and says how many it changed. It leaves the
accounts that already hold one alone, so running it twice changes
nothing the second time. Choose the role with care, because it goes to
every account holding none rather than to one you pick.

Like every command that changes an existing account, it acts as an
admin you name with `-as`, and it only shows what it would change until
you add `-yes`:

```sh
docker compose exec alphone /alphone \
  account:grant-role -role member -as admin@example.com
```

```text
would grant member to 2 accounts
alphone: dry run, nothing changed, pass -yes to apply
```

Run the same line with `-yes` at the end to give the role. The acting
account must be enabled, activated and hold a role that manages users,
such as admin. When nobody holds a role, no account can act, and the
command stops with
`the account admin@example.com holds no role, so it lacks manage_users`.
Create a new admin with `account:create-admin`, as in
[Install](/self-hosting/install/#4-start-it-and-create-the-admin-login),
and act as it:

```sh
docker compose exec alphone /alphone account:create-admin \
  -email rescue@example.com -name "Rescue Admin" -role admin
docker compose exec alphone /alphone \
  account:grant-role -role member -as rescue@example.com -yes
docker compose exec alphone /alphone \
  account:role admin@example.com admin -as rescue@example.com -yes
```

The last line gives one former admin the admin role back. Repeat it for
each of them. Every applied change is recorded, and `account:records`
lists who made it.

## Backup scenario

A nightly `pg_dump` covers a single-server install. Save this as
`/srv/alphone/backup.sh` and make it executable:

```sh
#!/bin/sh
# Dump the AlphOne database and prune dumps older than 14 days.
set -eu

here="$(cd "$(dirname "$0")" && pwd)"
dir="$here/backups"
mkdir -p "$dir"

docker compose -f "$here/compose.yaml" exec -T postgres \
	pg_dump -U alphone alphone | gzip >"$dir/alphone-$(date +%F-%H%M).sql.gz"

find "$dir" -name '*.sql.gz' -mtime +14 -delete
```

Schedule it daily:

```sh
( crontab -l 2>/dev/null; echo '0 3 * * * /srv/alphone/backup.sh' ) | crontab -
```

Copy the `backups/` directory somewhere off the server on your own
schedule. A backup that lives only next to the database it protects is
half a backup.

## Restoring

Stop the app, recreate the database, replay the dump, start the app:

```sh
cd /srv/alphone
docker compose stop alphone
docker compose exec -T postgres psql -U alphone -d postgres \
  -c 'DROP DATABASE alphone' -c 'CREATE DATABASE alphone OWNER alphone'
gunzip -c backups/alphone-<date>.sql.gz | \
  docker compose exec -T postgres psql -U alphone alphone
docker compose start alphone
```
