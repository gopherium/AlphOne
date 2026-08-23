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
able to promote anyone back. `alphone grantrole -role member` gives a
role to every account holding none, and says how many it changed. It
leaves the accounts that already hold one alone, so running it twice
changes nothing the second time. Choose the role with care, because it
goes to every account holding none rather than to one you pick.

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
