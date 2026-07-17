---
title: Updates and backups
description: How releases reach your server, how to update automatically or roll back, and how to back up and restore the database.
---

## How releases work

Every AlphOne release is a git tag (`v0.1.0`, `v0.2.0`, ...) that
publishes a container image to
`ghcr.io/gopherium/alphone` under three tags:

- the version, e.g. `:0.1.0`
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
    image: ghcr.io/gopherium/alphone:0.1.0
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
