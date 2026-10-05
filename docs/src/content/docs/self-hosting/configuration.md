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
| `ALPHONE_TRUSTED_PROXIES` | no | unset | Comma-separated CIDR ranges allowed to set `X-Forwarded-For`, e.g. `172.18.0.0/16`. Only addresses in these ranges are trusted when the login rate limiter resolves the client IP. Unset, the direct peer address is used. **Set this whenever AlphOne runs behind a reverse proxy**, or all visitors share one rate-limit bucket. Each entry must be CIDR notation. A bare IP is rejected at startup. |
| `ALPHONE_DEV_GRAPHIQL` | no | unset | Any non-empty value serves the interactive GraphiQL page on `GET /api/graphql`. Development only. |

## Lists, toasts and formats

| Variable | Purpose |
| --- | --- |
| `ALPHONE_GRAPH_PAGE_SIZE` | Rows a core graph list (`contacts`, `tasks`, `contactPage`) answers when the caller names no size. Defaults to 50. AlphOne will not start if the value is not a positive whole number or is above `ALPHONE_GRAPH_PAGE_CAP`. |
| `ALPHONE_GRAPH_PAGE_CAP` | The most rows one core graph list answers. Defaults to 200. A request for more is refused, and AlphOne will not start if the value is not a positive whole number or is above 1250. Reading even one field of each row costs 2 a row, so 1250 rows is the largest page that fits the query cost limit of 2500. A wider read of a large page can still be refused by that limit, see [Limits](/reference/graphql-api/#limits). |
| `ALPHONE_TOAST_DURATION` | How long a confirmation toast stays on screen, written as a duration such as `6s` or `1500ms`. Defaults to `6s`. AlphOne will not start unless the value is whole milliseconds from `1ms` to `2147483647ms`. |
| `ALPHONE_LIST_PAGE_SIZES` | The page sizes a list screen offers, comma separated. Defaults to `10,20,50,100`. AlphOne will not start unless each size is a positive whole number, listed once from the smallest up, and none is above `ALPHONE_GRAPH_PAGE_CAP`. |
| `ALPHONE_LIST_PAGE_SIZE` | The page size a list screen opens on. Defaults to 20. AlphOne will not start unless it is one of `ALPHONE_LIST_PAGE_SIZES`. |
| `ALPHONE_FORMAT_LOCALE` | The locale every screen writes dates, times, numbers and money in, whatever language a reader picked for the interface. Defaults to `es-ES`, which writes a date as 30/09/2026, a time as 09:05 on a 24 hour clock, a number as 1.234,56 and an amount as 1.234,56 €. Numbers always group their thousands, four digit ones too. Names of days, such as Thursday or Today, stay in the interface language. AlphOne will not start unless the value is a BCP 47 language tag that names a language, such as `en-GB` or `de-DE`. A tag with no language, such as `und` or the private use tag `x-foo`, stops it too. |

## Timeouts and shutdown

Each value is written as a duration such as `30s`, `1500ms` or `2m`.
AlphOne will not start unless every value is a duration above zero. A
value in words stops it with an error such as
`ALPHONE_SHUTDOWN_GRACE: must be a duration like 30s, got "soon"`, and a
zero stops it with `must stand above zero, got "0s"`.

A stop runs the three shutdown graces one after the other, so with the
defaults it can take up to 20 seconds. Whatever stops AlphOne must wait
longer than the three added together. The Docker Compose file in
[Install](/self-hosting/install/) waits 25 seconds, so raise its
`stop_grace_period` whenever you raise a grace.

| Variable | Purpose |
| --- | --- |
| `ALPHONE_HTTP_READ_HEADER_TIMEOUT` | How long a client has to send the headers of a request. Defaults to `10s`. |
| `ALPHONE_HTTP_READ_TIMEOUT` | How long a client has to send a whole request, its body included. Defaults to `30s`. A request still arriving after that is cut off, so raise it when large uploads come over slow connections. |
| `ALPHONE_HTTP_IDLE_TIMEOUT` | How long an idle connection waits for its next request before AlphOne closes it. Defaults to `2m`. |
| `ALPHONE_SHUTDOWN_GRACE` | How long running requests get to finish once a stop begins. Defaults to `10s`. The requests still running after that are cancelled. |
| `ALPHONE_SHUTDOWN_CANCEL_GRACE` | How long the cancelled requests get to end before AlphOne closes their connections. Defaults to `5s`. |
| `ALPHONE_SHUTDOWN_STOP_GRACE` | How long the plugins get to stop once the requests are over. Defaults to `5s`. |

## Webhooks

AlphOne refuses to deliver a [webhook](/reference/webhooks/) to an
internal address unless you allow it here. Internal means loopback, the
private network ranges, the shared and reserved ranges, link-local and
multicast addresses, and their IPv6 counterparts. Public addresses are
reached on any port, apart from the cloud host addresses listed below.

| Variable | Purpose |
| --- | --- |
| `ALPHONE_WEBHOOK_ALLOWED_HOSTS` | The internal addresses webhooks may still reach, comma separated. Empty by default, which refuses them all. An entry is a CIDR range such as `127.0.0.1/32`, or a host name with a port such as `n8n:5678`. |

The two kinds of entry match at different moments:

- A host name with a port matches the host and port written in the
  webhook URL, before the name is looked up. `n8n:5678` opens
  `http://n8n:5678/...` whatever address the name points to, and
  nothing else on that network. Prefer it for one service on the same
  Docker network, because that network usually holds the database too.
  A URL that names no port uses 80 for `http` and 443 for `https`, so
  `https://hooks.internal/receive` needs the entry `hooks.internal:443`.
- A CIDR range matches the address AlphOne is about to connect to, after
  the lookup. `127.0.0.1/32` opens every port on `127.0.0.1`, whatever
  name the webhook URL used to get there.

Link-local addresses, `169.254.0.0/16` and `fe80::/10`, stay refused
even when an entry lists them. So do the cloud metadata and host
addresses that sit outside those ranges: `100.100.100.200` (Alibaba
Cloud), `168.63.129.16` (Azure), `192.0.0.192` (Oracle), `fd00:ec2::23`
and `fd00:ec2::254` (AWS), `fd20:ce::254` (Google Cloud),
`fd00:a9fe:a9fe::1` (Akamai) and `fd00:42::42` (Scaleway). Azure's
address is public, and it is refused all the same. The addresses next
to them stay open to an entry that lists their range.

On a network that reaches IPv4 through a NAT64 translator, an address in
`64:ff9b::/96` is judged as the IPv4 address it carries. A public one
passes with no entry, and an internal one needs its IPv4 range listed.

AlphOne will not start unless every entry is one of the two kinds. A
host name without a port, such as `n8n`, stops it with:

```text
ALPHONE_WEBHOOK_ALLOWED_HOSTS: must list CIDR ranges such as 127.0.0.1/32 or host names with a port such as n8n:5678, got "n8n"
```

A refused delivery counts as a failed attempt and is retried like any
other. Each one leaves a warning in the log,
`refusing a webhook delivery to an internal address`, naming the host.
Webhook deliveries ignore `HTTP_PROXY` and `HTTPS_PROXY` and always
connect to the subscriber directly.

## Cross-origin writes

AlphOne refuses a write that a browser sends from a page at another
origin, so a page elsewhere cannot make a signed-in person's browser
change anything. Reads are never refused. A request that carries
neither `Sec-Fetch-Site` nor `Origin` passes, so API tokens, scripts,
n8n, AI agents over MCP and the WhatsApp webhook work as before. There
is no setting to turn the check off.

A browser write is judged in one of two ways:

- A current browser says where the page stood in `Sec-Fetch-Site`. A
  write from AlphOne's own pages passes, anything else is refused.
- A browser from before 2023 does not send that header and is judged by
  its `Origin`. Its host and port must match the `Host` header of the
  write. The scheme is not compared.

That second way is why a reverse proxy in front of AlphOne should pass
the visitor's `Host` header through unchanged. AlphOne never reads
`X-Forwarded-Host` for this check. Caddy and Traefik keep the `Host`
header by default, and nginx needs `proxy_set_header Host $http_host`,
which keeps the port as well. Have the proxy send
`Strict-Transport-Security` as well, so those older browsers never load
one of your pages over plain `http`.

A refused write answers HTTP 403 with this body, and the app tells the
person that the request came from a page on another site:

```json
{ "error": "cross-origin request refused", "code": "request_cross_origin" }
```

The log gets a warning, `write refused`, with the method, the path, the
`Host` header, the `Origin` and `Sec-Fetch-Site` the browser sent, and
one of two reasons:

- `fetch-site` when the browser said in `Sec-Fetch-Site` that the page
  stood at another origin, a sibling subdomain included
- `origin` when the browser sent only `Origin` and its host and port do
  not match the `Host` header

## Fields plugin

| Variable | Purpose |
| --- | --- |
| `ALPHONE_FIELDS_ENTRIES_MAX` | The most entries one repeater field holds on a contact. Defaults to 500. An add past the cap is refused, and AlphOne will not start if the value is not a whole number between 1 and 2147483647. |

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
  `login` answers with an error whose `extensions.code` is
  `RATE_LIMITED` and whose `extensions.retryAfter` is the wait in
  seconds, see [errors](/reference/graphql-api/#errors). The limit is per
  address and there is no per-account lockout, so an attacker spreading
  guesses across many addresses is bounded only by password strength and
  the argon2id hashing cost. Passwords must be at least 12 characters.
  Until multi-factor authentication ships, a long unique password is the
  account-side defense.
- **The session cookie** is `HttpOnly`, `Secure`, `SameSite=Lax`, with
  the `__Host-` prefix. This is why [HTTPS is
  mandatory](/self-hosting/install/) in production. On top of the
  cookie, writes a browser sends from another origin are refused, see
  [Cross-origin writes](#cross-origin-writes).
