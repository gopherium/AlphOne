---
title: Install
description: Run AlphOne on your own server with Docker Compose, a PostgreSQL container, and any HTTPS reverse proxy.
---

AlphOne ships as a single container image. The Go binary inside serves
both the JSON API and the built frontend, so a production deployment is
exactly two containers: AlphOne and PostgreSQL. You bring the third
piece, an HTTPS reverse proxy. The examples below use
[Caddy](https://caddyserver.com/) running in Docker, but any proxy that
terminates TLS works the same way.

```text
internet ── your proxy (TLS) ──► alphone :8080 ──► postgres
```

:::caution[HTTPS is not optional]
The session cookie uses the `__Host-` prefix and the `Secure` attribute.
Browsers refuse to store it over plain HTTP, so a deployment without TLS
lets nobody log in. Terminate TLS at the proxy. AlphOne itself speaks
plain HTTP on the internal Docker network only.
:::

## 1. Lay down the files

Pick a directory on the server, for example `/srv/alphone/`, and create
`compose.yaml` in it:

```yaml
name: alphone

services:
  alphone:
    image: ghcr.io/gopherium/alphone:latest
    restart: unless-stopped
    stop_grace_period: 25s
    env_file: .env
    environment:
      ALPHONE_ADDR: "0.0.0.0:8080"
      ALPHONE_DATABASE_URL: "postgres://alphone:${POSTGRES_PASSWORD}@postgres:5432/alphone?sslmode=disable"
      ALPHONE_TRUSTED_PROXIES: "${ALPHONE_TRUSTED_PROXIES:?set in .env to the subnet of your proxy docker network}"
    labels:
      # Optional, for What's Up Docker. Harmless without it.
      wud.watch: "true"
      wud.watch.digest: "true"
    depends_on:
      postgres:
        condition: service_healthy
    networks:
      - internal
      - caddy

  postgres:
    image: postgres:18
    restart: unless-stopped
    labels:
      # Never auto-update the database. A major-version jump breaks its data.
      wud.watch: "false"
    environment:
      POSTGRES_USER: alphone
      POSTGRES_PASSWORD: "${POSTGRES_PASSWORD}"
      POSTGRES_DB: alphone
    volumes:
      - pgdata:/var/lib/postgresql
    healthcheck:
      test: ["CMD-SHELL", "pg_isready -U alphone -d alphone"]
      interval: 5s
      timeout: 3s
      retries: 12
    networks:
      - internal

networks:
  internal:
  caddy:
    external: true
    # Set this to the docker network your reverse proxy is attached to.
    name: caddy

volumes:
  pgdata:
```

`stop_grace_period: 25s` is there because a stop can take up to 20
seconds while AlphOne lets running requests and plugins finish, and
Docker kills a container after 10 by default.

Set the external network `name:` to the Docker network your proxy lives
on. Find it with:

```sh
docker inspect <proxy-container> -f '{{json .NetworkSettings.Networks}}'
```

## 2. Create the environment file

Create `.env` next to `compose.yaml`, with permissions `600`:

```ini
POSTGRES_PASSWORD=<a strong random password>
ALPHONE_TRUSTED_PROXIES=<the subnet of your proxy docker network>

# Only needed once you connect WhatsApp. See the Meta setup guide.
ALPHONE_WHATSAPP_VERIFY_TOKEN=<from Meta>
ALPHONE_WHATSAPP_APP_SECRET=<from Meta>
ALPHONE_WHATSAPP_ACCESS_TOKEN=<from Meta>
ALPHONE_WHATSAPP_PHONE_NUMBER_ID=<from Meta>

# Only needed when a webhook receiver, such as n8n, shares a network with AlphOne.
# ALPHONE_WEBHOOK_ALLOWED_HOSTS=n8n:5678
```

`ALPHONE_TRUSTED_PROXIES` deserves a moment of attention. The login rate
limiter counts failed attempts per client address, and behind a proxy the
real address arrives in the `X-Forwarded-For` header. AlphOne only trusts
that header from the CIDR ranges you list here. Left unset, every visitor
would share the proxy's address and one rate-limit bucket, so ten failed
logins by anyone would lock everyone out. Find the subnet with:

```sh
docker network inspect <proxy-network> -f '{{range .IPAM.Config}}{{.Subnet}}{{end}}'
```

Keep that network limited to the proxy and the apps it fronts, because
any container attached to it can set the header.

AlphOne refuses to deliver webhooks to internal addresses, and every
container on your Docker networks has one. If n8n or another webhook
receiver runs next to AlphOne and its webhook URLs name it on a shared
network, such as `http://n8n:5678/webhook/...`, allow that one service
with `ALPHONE_WEBHOOK_ALLOWED_HOSTS=n8n:5678`. A receiver reached at a
public `https` address needs nothing. See
[Webhooks](/self-hosting/configuration/#webhooks) for the format.

Without the WhatsApp variables the plugin runs inert: its screens exist
but no webhook verifies and no message sends. Fill them in whenever you
are ready, following [Meta setup](/whatsapp/meta-setup/).

## 3. Point your proxy at it

For a dockerized Caddy, add a site block and reload:

```caddy
alphone.example.com {
	reverse_proxy alphone:8080
}
```

```sh
docker exec <caddy-container> caddy reload --config /etc/caddy/Caddyfile
```

Caddy obtains and renews the certificate automatically, and keeps the
`Host` header the visitor typed.

For nginx or Traefik, proxy the domain to `alphone:8080` and make sure
the proxy sets `X-Forwarded-For` and keeps the visitor's `Host` header.
Traefik keeps it by default. With nginx, add:

```nginx
proxy_set_header Host $http_host;
proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
```

Use `$http_host` rather than `$host`. It passes the port too, which
matters when AlphOne answers on a port other than 443.

The `Host` header matters only for browsers from before 2023, which do
not send `Sec-Fetch-Site`. AlphOne compares their `Origin` with `Host`,
so a proxy that renames the host makes AlphOne refuse their saves, see
[Cross-origin writes](/self-hosting/configuration/#cross-origin-writes).

Sending `Strict-Transport-Security` from the proxy is your choice. That
comparison never looks at the scheme, and the header closes that gap by
keeping browsers on `https` for your domain. It also commits you. A
browser that has seen it uses only `https` for your domain, and lets
nobody click past a certificate warning there, until its time runs out.
If you choose it, start with a short time such as `max-age=300` and
raise it once everything works. Sending `max-age=0` later, over valid
`https`, tells browsers to forget it. A browser ignores the header on
plain `http`.

In Caddy, add `header Strict-Transport-Security "max-age=300"` to the
site block. With nginx, add
`add_header Strict-Transport-Security "max-age=300" always;`. In
Traefik, put the header in `customResponseHeaders` on a headers
middleware. Its `stsSeconds` option drops the header at 0, so it could
never send `max-age=0`.

## 4. Start it and create the admin login

```sh
cd /srv/alphone
docker compose up -d
```

The image runs `alphone serve` when the service names no command.
`serve` applies every migration before it listens, so the database is
ready on first boot. A `command:` of your own replaces it, so that
command must be `serve`.

Create the first account:

```sh
docker compose exec alphone /alphone account:create-admin \
  -email you@example.com -name "Your Name" -role admin
```

At the `Password:` prompt, type a password of at least 12 characters and
press Enter. The prompt does not hide what you type. The command answers
`created user you@example.com`. Then open `https://your-domain` and log
in.

The command reads the password from the first line of its input, so a
script can pipe it in instead. Add `-T`, so Docker reads the input from
the pipe rather than from a terminal:

```sh
printf '%s\n' "$ADMIN_PASSWORD" | docker compose exec -T alphone /alphone \
  account:create-admin -email you@example.com -name "Your Name" -role admin
```

`-role admin` makes an admin, so this account can create the rest of your
colleagues. Everyone it creates arrives as a member and works the product
without managing users. Promote one from the Users screen when you want a
second admin. See [Roles](/reference/graphql-api/#roles) for what each tier
may do.

## Next steps

- [Configuration](/self-hosting/configuration/) lists every environment
  variable.
- [Commands](/self-hosting/commands/) lists every command the binary
  takes, such as the ones that change an account or mint an API token.
- [Updates and backups](/self-hosting/updates-and-backups/) covers
  staying current and not losing data.
- [Meta setup](/whatsapp/meta-setup/) connects your WhatsApp number. Its
  webhook endpoint is
  `https://your-domain/api/plugins/whatsapp/webhook`.
