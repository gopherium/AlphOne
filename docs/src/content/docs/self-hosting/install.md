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
lets nobody log in. Terminate TLS at the proxy; AlphOne itself speaks
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
      - pgdata:/var/lib/postgresql/data
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

Caddy obtains and renews the certificate automatically. For nginx or
Traefik, proxy the domain to `alphone:8080` and make sure the proxy sets
`X-Forwarded-For`.

## 4. Start it and create the admin login

```sh
cd /srv/alphone
docker compose up -d
```

Migrations run automatically on container start, so the database is
ready on first boot. Create the first account:

```sh
docker compose exec alphone /alphone createadmin \
  -email you@example.com -name "Your Name"
```

Type a password of at least 12 characters at the prompt, then open
`https://your-domain` and log in.

## Next steps

- [Configuration](/self-hosting/configuration/) lists every environment
  variable.
- [Updates and backups](/self-hosting/updates-and-backups/) covers
  staying current and not losing data.
- [Meta setup](/whatsapp/meta-setup/) connects your WhatsApp number. Its
  webhook endpoint is
  `https://your-domain/api/plugins/whatsapp/webhook`.
