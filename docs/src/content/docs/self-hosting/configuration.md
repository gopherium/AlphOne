---
title: Configuration
description: Every environment variable AlphOne reads, its default, and what it controls.
---

AlphOne is configured through environment variables. The binary also
loads a `.env` file from its working directory at startup, so the same
variables can live in a file next to it. Real environment variables take
precedence over `.env` entries, which is how the container setup in
[Install](/self-hosting/install/) works. The repository ships
[`.env.example`](https://github.com/gopherium/AlphOne/blob/main/.env.example)
as a commented template.

Database migrations for the core, the auth layer, and every plugin run
automatically at startup, so pointing a new version at an existing
database is all an upgrade takes.

## Core

| Variable | Required | Default | Purpose |
| --- | --- | --- | --- |
| `ALPHONE_DATABASE_URL` | yes | none | PostgreSQL connection string, e.g. `postgres://user:pass@host:5432/alphone?sslmode=disable`. |
| `ALPHONE_ADDR` | no | `localhost:8080` | Listen address. The container image sets `0.0.0.0:8080`. |
| `ALPHONE_WEB_DIR` | no | unset | Directory holding the built frontend, served for all non-API paths. The container image sets `/web`. Unset, only the API is served, which suits development behind Vite. |
| `ALPHONE_TRUSTED_PROXIES` | no | unset | Comma-separated CIDR ranges allowed to set `X-Forwarded-For`, e.g. `172.18.0.0/16`. Only addresses in these ranges are trusted when the login rate limiter resolves the client IP. Unset, the direct peer address is used. **Set this whenever AlphOne runs behind a reverse proxy**, or all visitors share one rate-limit bucket. Each entry must be CIDR notation; a bare IP is rejected at startup. |

## WhatsApp plugin

All optional. Without them the plugin runs inert: screens exist, but no
webhook verifies and no message sends. Values come from your Meta app,
see [Meta setup](/whatsapp/meta-setup/).

| Variable | Purpose |
| --- | --- |
| `ALPHONE_WHATSAPP_VERIFY_TOKEN` | The token Meta echoes during webhook verification. You invent it and paste the same value on both sides. |
| `ALPHONE_WHATSAPP_APP_SECRET` | The app secret, used to check the signature Meta sends with every webhook delivery. |
| `ALPHONE_WHATSAPP_ACCESS_TOKEN` | Bearer token for sending messages through the Graph API. |
| `ALPHONE_WHATSAPP_PHONE_NUMBER_ID` | The phone number ID (not the phone number itself) messages are sent from. |
| `ALPHONE_WHATSAPP_GRAPH_URL` | Graph API base URL. Defaults to `https://graph.facebook.com/v23.0`. Only override it for testing. |
| `ALPHONE_WHATSAPP_MEDIA_MAX_BYTES` | Largest inbound attachment stored, in bytes. Defaults to 26214400 (25 MiB), enough for every WhatsApp media type except large documents. Attachments over the cap appear in the thread as a named chip without a download. |

## Behavior worth knowing

- **Sessions** last 30 days, are stored server-side, and expired ones are
  garbage-collected hourly. Disabling a user revokes all of their
  sessions immediately.
- **Media attachments** (photos, voice notes, videos, documents,
  stickers) are downloaded from Meta shortly after each message arrives
  and stored in the PostgreSQL database, so a database backup contains
  the complete conversation history including attachments. Expect backup
  size to grow with media traffic.
- **Delivery status** for outbound replies (sent, delivered, read) is
  updated live from Meta's status webhooks and shown as ticks on each
  message. Failed deliveries, such as replying outside WhatsApp's
  24-hour customer service window, are surfaced on the message with an
  explanation. Statuses arrive through the same `messages` webhook
  field, so no extra Meta configuration is needed.
- **Login rate limiting** allows 10 failed attempts per client address
  per minute. Successful logins never consume the budget. Over the limit
  the API answers `429` with a `Retry-After` header.
- **The session cookie** is `HttpOnly`, `Secure`, `SameSite=Lax`, with
  the `__Host-` prefix. This is why [HTTPS is
  mandatory](/self-hosting/install/) in production.
