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

When a variable holds a value AlphOne refuses:

- `alphone check` names it, without connecting to the database.
- `serve` will not start, unless the variable is one of the three under
  [Account records and tokens](#account-records-and-tokens), which only
  the commands read.
- A refused value under [Mail](#mail), [Workspaces](#workspaces),
  [Fields plugin](#fields-plugin) or [WhatsApp plugin](#whatsapp-plugin),
  or in `ALPHONE_SHUTDOWN_STOP_GRACE`, also stops the account commands
  that take `-as`, before they change anything. It stops `migrate`,
  `seed -yes` and `account:create-admin` too, but only once they have
  applied the account, record and core migrations.

[Commands](/self-hosting/commands/) describes every command.

Database migrations for the core, the auth layer, the command records
and every plugin run automatically when `serve` starts, so pointing a
new version at an existing database is all an upgrade takes.
`alphone migrate` runs them without starting the server. Each run waits
for one database lock, so a starting server and a command never migrate
together. The wait is fixed: AlphOne asks for the lock every five
seconds and gives up after five minutes.

## Core

| Variable | Required | Default | Purpose |
| --- | --- | --- | --- |
| `ALPHONE_DATABASE_URL` | yes | none | PostgreSQL connection string, e.g. `postgres://user:pass@host:5432/alphone?sslmode=disable`. |
| `ALPHONE_ADDR` | no | `localhost:8080` | Listen address. The container image sets `0.0.0.0:8080`. |
| `ALPHONE_WEB_DIR` | no | unset | Directory holding the built frontend, served for all non-API paths. The container image sets `/web`. Unset, only the API is served, which suits development behind Vite. |
| `ALPHONE_TRUSTED_PROXIES` | no | unset | Comma-separated CIDR ranges allowed to set `X-Forwarded-For`, e.g. `172.18.0.0/16`. Only addresses in these ranges are trusted when the login rate limiter resolves the client IP. Unset, the direct peer address is used. **Set this whenever AlphOne runs behind a reverse proxy**, or all visitors share one rate-limit bucket. Each entry must be CIDR notation. A bare IP is refused. |
| `ALPHONE_DEV_GRAPHIQL` | no | unset | Any non-empty value serves the interactive GraphiQL page on `GET /api/graphql`. Development only. |

## Mail

AlphOne mails invitations and password reset links through a relay you
name. Without `ALPHONE_SMTP_HOST` it sends no mail. An invitation then
shows its activation link on screen for you to pass on, and a password
reset request sends nothing. `ALPHONE_SMTP_PORT`,
`ALPHONE_SMTP_USERNAME`, `ALPHONE_SMTP_PASSWORD`, `ALPHONE_SMTP_FROM`
and `ALPHONE_SMTP_TLS` are refused without a host, with an error such as
`ALPHONE_SMTP_PORT is set but ALPHONE_SMTP_HOST is not`.
`ALPHONE_PUBLIC_URL` and `ALPHONE_MAIL_TEMPLATE_DIR` are read only with
a host.

| Variable | Purpose |
| --- | --- |
| `ALPHONE_SMTP_HOST` | The host name of the mail relay. Unset, AlphOne sends no mail. |
| `ALPHONE_SMTP_PORT` | The port of the relay. Defaults to 587. Must be a whole number from 1 to 65535. |
| `ALPHONE_SMTP_USERNAME` | The user name AlphOne logs in to the relay with. Set it together with `ALPHONE_SMTP_PASSWORD`, or leave both unset to send without logging in. |
| `ALPHONE_SMTP_PASSWORD` | The password for that user name. |
| `ALPHONE_SMTP_FROM` | The address every mail is sent from, such as `crm@example.com`. Required with a host. |
| `ALPHONE_SMTP_TLS` | How the connection to the relay is secured with STARTTLS. `mandatory`, the default, refuses a relay that does not offer it. `opportunistic` uses it when the relay offers it, and `none` never does. A user name and password need `mandatory`. |
| `ALPHONE_PUBLIC_URL` | The address people reach AlphOne at, such as `https://crm.example.com`. The link in every mail leads back to it. Required with a host. It must be an `http` or `https` address of a site root, with no path, query or fragment. |
| `ALPHONE_MAIL_TEMPLATE_DIR` | A directory of your own mail templates. A file named `invite.tmpl` or `reset.tmpl` there replaces the built-in template of that name. Unset, the built-in ones are used. AlphOne refuses a directory that does not exist. |

## Invitations and password resets

A duration is written like `1h` or `30m` and must be above zero. A
count must be a whole number above zero.

| Variable | Purpose |
| --- | --- |
| `ALPHONE_INVITE_TTL` | How long the activation link of an invitation lives. Defaults to `168h`, seven days. |
| `ALPHONE_RESET_TTL` | How long a password reset link lives. Defaults to `1h`. |
| `ALPHONE_RESET_ATTEMPTS` | How many reset requests one client address may make within one reset link lifetime. Defaults to 3. A request over the limit is refused as rate limited. |
| `ALPHONE_RESET_LINKS` | How many reset links one account may hold at once. Defaults to 3. Once it holds that many, a further request sends none until one is used or expires. |
| `ALPHONE_RESET_COOLDOWN` | The shortest gap between two reset mails to one address. Defaults to `1m`. A request inside the gap sends nothing. |

## Workspaces

Every account works in a workspace. These variables matter once a
deployment serves more than one.

| Variable | Purpose |
| --- | --- |
| `ALPHONE_TENANT_MACHINE_GRACE` | How long a deactivated workspace keeps recording what a channel such as WhatsApp delivers, written as a duration. Defaults to `336h`, fourteen days. `0s` stops the recording as soon as the workspace is deactivated. |
| `ALPHONE_TENANTS_HELD` | How many workspaces' fields each server process keeps in memory. Defaults to 256. Must be a whole number above zero. |
| `ALPHONE_TENANTS_REFRESH` | How long a server process keeps a workspace's fields before reading them again. Defaults to `1m`. Must be a duration above zero. |

## Lists, toasts and formats

| Variable | Purpose |
| --- | --- |
| `ALPHONE_GRAPH_PAGE_SIZE` | Rows a core graph list (`contacts`, `tasks`, `contactPage`) answers when the caller names no size. Defaults to 50. `serve` will not start if the value is not a positive whole number or is above `ALPHONE_GRAPH_PAGE_CAP`. |
| `ALPHONE_GRAPH_PAGE_CAP` | The most rows one core graph list answers. Defaults to 200. A request for more is refused, and `serve` will not start if the value is not a positive whole number or is above 1250. Reading even one field of each row costs 2 a row, so 1250 rows is the largest page that fits the query cost limit of 2500. A wider read of a large page can still be refused by that limit, see [Limits](/reference/graphql-api/#limits). |
| `ALPHONE_TOAST_DURATION` | How long a confirmation toast stays on screen, written as a duration such as `6s` or `1500ms`. Defaults to `6s`. `serve` will not start unless the value is whole milliseconds from `1ms` to `2147483647ms`. |
| `ALPHONE_LIST_PAGE_SIZES` | The page sizes a list screen offers, comma separated. Defaults to `10,20,50,100`. `serve` will not start unless each size is a positive whole number, listed once from the smallest up, and none is above `ALPHONE_GRAPH_PAGE_CAP`. |
| `ALPHONE_LIST_PAGE_SIZE` | The page size a list screen opens on. Defaults to 20. `serve` will not start unless it is one of `ALPHONE_LIST_PAGE_SIZES`. |
| `ALPHONE_FORMAT_LOCALE` | The locale every screen writes dates, times, numbers and money in, whatever language a reader picked for the interface. Defaults to `es-ES`, which writes a date as 30/09/2026, a time as 09:05 on a 24 hour clock, a number as 1.234,56 and an amount as 1.234,56 €. Numbers always group their thousands, four digit ones too. Names of days, such as Thursday or Today, stay in the interface language. `serve` will not start unless the value is a BCP 47 language tag that names a language, such as `en-GB` or `de-DE`. A tag with no language, such as `und` or the private use tag `x-foo`, stops it too. |

## Timeouts and shutdown

Each value is written as a duration such as `30s`, `1500ms` or `2m`.
`serve` will not start unless every value is a duration above zero. A
value in words stops it with an error such as
`ALPHONE_SHUTDOWN_GRACE: must be a duration like 30s, got "soon"`, and a
zero stops it with `must stand above zero, got "0s"`. A refused
`ALPHONE_SHUTDOWN_STOP_GRACE` also stops the other commands listed at
the top of this page.

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
which keeps the port as well. Sending `Strict-Transport-Security` from
the proxy is optional. It keeps those older browsers from loading one of
your pages over plain `http`, and the [install guide](/self-hosting/install/)
explains what it commits you to.

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
| `ALPHONE_FIELDS_ENTRIES_MAX` | The most entries one repeater field holds on a contact. Defaults to 500. An add past the cap is refused. A value that is not a whole number between 1 and 2147483647 stops `serve` and the other commands listed at the top of this page. |

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
| `ALPHONE_WHATSAPP_CREDENTIALS_KEY` | A key of 32 bytes written as 64 hex characters, which seals a workspace's own WhatsApp access token in the database when a plugin stores one per workspace. Unset, no such token can be stored. AlphOne refuses a value that is not hex or does not hold 32 bytes. |

## Account records and tokens

Only the [commands](/self-hosting/commands/) read these. `serve`
ignores them, and `alphone check` names a refused value.

| Variable | Purpose |
| --- | --- |
| `ALPHONE_COMMAND_RECORD_TIMEOUT` | How long storing the record of one applied account change may take, written as a duration such as `5s`. Defaults to `5s`. A record that takes longer fails, and the command exits 1 with the change already made. The account commands that take `-as` refuse a value that is not a duration above zero. |
| `ALPHONE_COMMAND_RECORDS_LIMIT` | How many records `account:records` lists when `-limit` is left out. Defaults to 50. `account:records` refuses a value that is not a whole number above zero, unless `-limit` is given. |
| `ALPHONE_TOKEN_TTL_DAYS` | How many days a token minted with `token:create` lasts when `-ttl` is left out. Defaults to 90. `0` mints tokens that never expire. `token:create` refuses a value that is not a whole number from 0 to 106751, unless `-ttl` is given. |

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
