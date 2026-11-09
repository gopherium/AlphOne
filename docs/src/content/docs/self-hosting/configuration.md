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
  [Contacts](#contacts), [Webhooks](#webhooks), [Files](#files),
  [Encryption](#encryption), [Email plugin](#email-plugin),
  [Fields plugin](#fields-plugin) or [WhatsApp plugin](#whatsapp-plugin),
  in `ALPHONE_PUBLIC_URL`, `ALPHONE_GRAPH_PAGE_SIZE` or
  `ALPHONE_GRAPH_PAGE_CAP`, or in `ALPHONE_SHUTDOWN_STOP_GRACE`, also
  stops the account commands that take `-as`, before they change
  anything. It stops `migrate`, `seed -yes` and `account:create-admin`
  too, but only once they have applied the account, record, core and
  storage migrations.

[Commands](/self-hosting/commands/) describes every command.

Database migrations for the core, the auth layer, the command records,
the file storage and every plugin run automatically when `serve` starts,
so pointing a new version at an existing database is all an upgrade
takes.
`alphone migrate` runs them without starting the server. Each run waits
for one database lock, so a starting server and a command never migrate
together. The wait is fixed: AlphOne asks for the lock every five
seconds and gives up after five minutes.

## Core

| Variable | Required | Default | Purpose |
| --- | --- | --- | --- |
| `ALPHONE_DATABASE_URL` | yes | none | PostgreSQL connection string, e.g. `postgres://user:pass@host:5432/alphone?sslmode=disable`. Write a reserved sign in the user name or password percent encoded, `@` as `%40`, `/` as `%2F` and `%` as `%25`. The password ends at its first `@`, so a bare `@` in it stops `serve` at start. |
| `ALPHONE_ADDR` | no | `localhost:8080` | Listen address. The container image sets `0.0.0.0:8080`. |
| `ALPHONE_WEB_DIR` | no | unset | Directory holding the built frontend, served for all non-API paths. The container image sets `/web`. Unset, only the API is served, which suits development behind Vite. |
| `ALPHONE_PUBLIC_URL` | no | unset | The address people reach AlphOne at, such as `https://crm.example.com`. It must be an `http` or `https` address of a site root, with no path, query or fragment. Plugins get it with or without a mail relay, for example to send people back to AlphOne from another site. An invitation link that AlphOne shows for you to pass on starts with it too. Unset, that link holds only its path, such as `/activate?token=...`. A mail relay requires it, because the link in every mail leads back to it. A value AlphOne refuses stops `serve` and the other commands listed at the top of this page. |
| `ALPHONE_PRIVACY_URL` | no | unset | The address of your privacy policy, such as `https://example.com/privacy`. It shows as a link under the form on the sign-in, lost password, set password and reset password screens. It must be an `http` or `https` address with a host and no user or password, and it may carry a port from 1 to 65535, a path, a query and a fragment. Unset, no link shows. |
| `ALPHONE_TERMS_URL` | no | unset | The address of your terms of service, shown and checked the same way as `ALPHONE_PRIVACY_URL`. The two together, each counted as written in JSON with its quotes, must leave 256 bytes of `ALPHONE_GRAPH_ANONYMOUS_ANSWER_MAX_BYTES`, since the sign-in screens read them with no identity. |
| `ALPHONE_TRUSTED_PROXIES` | no | unset | Comma-separated CIDR ranges allowed to set `X-Forwarded-For`, e.g. `172.18.0.0/16`. Only addresses in these ranges are trusted when the login rate limiter resolves the client IP. A peer outside them is limited by its own address, whatever header it sends. A range written in IPv4-mapped form, such as `::ffff:172.18.0.0/112`, counts as the IPv4 range it covers, and one shorter than `/96` stops `serve`. Unset, the direct peer address is used. **Set this whenever AlphOne runs behind a reverse proxy**, or all visitors share one rate-limit bucket. Each entry must be CIDR notation. A bare IP is refused. |
| `ALPHONE_DEV_GRAPHIQL` | no | unset | Any non-empty value serves the interactive GraphiQL page on `GET /api/graphql`. Development only. |

## Mail

AlphOne mails invitations and password reset links through a relay you
name. Without `ALPHONE_SMTP_HOST` it sends no mail. An invitation then
shows its activation link on screen for you to pass on, and a password
reset request sends nothing. `ALPHONE_SMTP_PORT`,
`ALPHONE_SMTP_USERNAME`, `ALPHONE_SMTP_PASSWORD`, `ALPHONE_SMTP_FROM`
and `ALPHONE_SMTP_TLS` are refused without a host, with an error such as
`ALPHONE_SMTP_PORT is set but ALPHONE_SMTP_HOST is not`.
`ALPHONE_MAIL_TEMPLATE_DIR` is read only with a host. A relay also needs
[`ALPHONE_PUBLIC_URL`](#core), the address the link in every mail leads
back to.

| Variable | Purpose |
| --- | --- |
| `ALPHONE_SMTP_HOST` | The host name of the mail relay. Unset, AlphOne sends no mail. |
| `ALPHONE_SMTP_PORT` | The port of the relay. Defaults to 587. Must be a whole number from 1 to 65535. |
| `ALPHONE_SMTP_USERNAME` | The user name AlphOne logs in to the relay with. Set it together with `ALPHONE_SMTP_PASSWORD`, or leave both unset to send without logging in. |
| `ALPHONE_SMTP_PASSWORD` | The password for that user name. |
| `ALPHONE_SMTP_FROM` | The address every mail is sent from, such as `crm@example.com`. Required with a host. |
| `ALPHONE_SMTP_TLS` | How the connection to the relay is secured with STARTTLS. `mandatory`, the default, refuses a relay that does not offer it. `opportunistic` uses it when the relay offers it, and `none` never does. A user name and password need `mandatory`. |
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

## Contacts

Plugins look up the contacts behind many addresses at once, such as every
recipient of a mail, see [Contacts](/extending/contacts/).

| Variable | Purpose |
| --- | --- |
| `ALPHONE_CONTACT_MATCH_MAX` | The most addresses a plugin looks up in one call. It also caps one page of the addresses contacts gain, so a page always fits one lookup. Defaults to 100. A value that is not a whole number between 1 and 2147483647 stops `serve` and the other commands listed at the top of this page. |

## Lists, toasts and formats

| Variable | Purpose |
| --- | --- |
| `ALPHONE_GRAPH_PAGE_SIZE` | Rows a graph list answers when the caller names no size, the core lists (`contacts`, `tasks`, `contactPage`) and the plugin lists alike, such as the WhatsApp conversations, each page of a WhatsApp chat and the rows of an import. Defaults to 50. A value that is not a positive whole number or is above `ALPHONE_GRAPH_PAGE_CAP` stops `serve` and the other commands listed at the top of this page. |
| `ALPHONE_GRAPH_PAGE_CAP` | The most rows one graph list answers, core and plugin lists alike. Defaults to 200. A request for more is refused. A value that is not a positive whole number or is above 1250 stops `serve` and the other commands listed at the top of this page. Reading even one field of each row costs 2 a row, so 1250 rows is the largest page that fits the query cost limit of 2500. A wider read of a large page can still be refused by that limit, see [Limits](/reference/graphql-api/#limits). |
| `ALPHONE_TOAST_DURATION` | How long a confirmation toast with no button stays on screen, written as a duration such as `6s` or `1500ms`. Defaults to `6s`. `serve` will not start unless the value is whole milliseconds from `1ms` to `2147483647ms`. |
| `ALPHONE_TOAST_ACTION_DURATION` | How long a confirmation toast with a button, such as Undo, stays on screen, written as a duration such as `10s` or `1500ms`. Defaults to `10s`. `serve` will not start unless the value is whole milliseconds from `1ms` to `2147483647ms`. |
| `ALPHONE_LIST_PAGE_SIZES` | The page sizes a list screen offers, comma separated. Defaults to `10,20,50,100`. `serve` will not start unless each size is a positive whole number, listed once from the smallest up, and none is above `ALPHONE_GRAPH_PAGE_CAP`. |
| `ALPHONE_LIST_PAGE_SIZE` | The page size a list screen opens on, and the first page of a repeater on a contact. Defaults to 20. `serve` will not start unless it is one of `ALPHONE_LIST_PAGE_SIZES`. |
| `ALPHONE_FORMAT_LOCALE` | The locale every screen writes dates, times, numbers and money in, whatever language a reader picked for the interface. Defaults to `es-ES`, which writes a date as 30/09/2026, a time as 09:05 on a 24 hour clock, a number as 1.234,56 and an amount as 1.234,56 €. Numbers always group their thousands, four digit ones too. Names of days, such as Thursday or Today, stay in the interface language. The dates in lists, such as Created in Users, follow the interface language too, as Oct 13, 2026 5:04 pm in English or 13 Oct 2026 17:04 in Spanish. `serve` will not start unless the value is a BCP 47 language tag that names a language, such as `en-GB` or `de-DE`. A tag with no language, such as `und` or the private use tag `x-foo`, stops it too. |

## Graph limits

Each count and byte size is a positive whole number, and each time is a
duration such as `60s` or `5m`. `serve` will not start on a zero, a
value in words, or a byte size written with a unit such as `1MiB`.

| Variable | Purpose |
| --- | --- |
| `ALPHONE_GRAPH_OPERATIONS_PER_USER` | How many graph operations one caller runs at once. Defaults to 20. One more is answered `429` with a `Retry-After` header. |
| `ALPHONE_GRAPH_OPERATION_TIMEOUT` | How long one graph operation may run before it is cancelled. Defaults to `60s`. Each write of an operation's answer also gets this long to reach the caller, and a caller who does not read it in time loses the connection. |
| `ALPHONE_GRAPH_BODY_MAX_BYTES` | The largest JSON graph request body, in bytes. Defaults to 1048576, which is 1 MiB. |
| `ALPHONE_GRAPH_UPLOAD_MAX_BYTES` | The largest multipart graph request body, an upload with its form, in bytes. Defaults to 6291456, which is 6 MiB. The importer still holds its file to 5 MiB, whatever this value is. |
| `ALPHONE_GRAPH_RETRY_AFTER` | The wait the `Retry-After` header names when every operation slot of a caller is taken. Defaults to `1s`. |
| `ALPHONE_GRAPH_ANONYMOUS_MAX_BYTES` | The largest graph request body from a caller who is not signed in, in bytes. Defaults to 16384, which is 16 KiB. The longest valid sign in, a 254 character address with a 1024 character password, is under 7000 bytes, and about 15500 bytes from a client that escapes every character. Such a caller sends JSON only. A multipart request or a larger body from one gets the `UNAUTHENTICATED` error before it takes a slot. |
| `ALPHONE_GRAPH_ANONYMOUS_MAX_TOKENS` | The most tokens the query of a caller who is not signed in holds. A token is one name, value, mark such as `{` or `:`, or comment. Defaults to 64. The largest query a screen sends before sign in, the sign in or the one accepting an invitation, holds 38. A longer query gets the `UNAUTHENTICATED` error before it takes a slot. |
| `ALPHONE_GRAPH_ANONYMOUS_QUERY_MAX_BYTES` | The largest query a caller who is not signed in sends, in bytes. Defaults to 1024. The largest query a screen sends before sign in is 212 bytes. Send credentials as variables, since a long password written into the query goes over this cap. A larger query gets the `UNAUTHENTICATED` error before it takes a slot. |
| `ALPHONE_GRAPH_ANONYMOUS_ANSWER_MAX_BYTES` | The largest answer a caller who is not signed in gets, in bytes. Defaults to 16384, which is 16 KiB. Such an answer is held in memory and its slot freed before it is sent, so a caller who reads slowly never holds a slot. A sign in answer is under 1000 bytes. A larger answer is replaced by the `UNAUTHENTICATED` error. It must hold 256 bytes beside `ALPHONE_PRIVACY_URL` and `ALPHONE_TERMS_URL`, each counted as written in JSON with its quotes, so at least 260 when neither is set, or `serve` will not start. |
| `ALPHONE_GRAPH_ANONYMOUS_PER_IP` | How many graph requests callers who are not signed in run at once from one client address. Defaults to 5. A signed out screen sends at most two at once, the locale and a sign in. The address is the one `ALPHONE_TRUSTED_PROXIES` names, so behind a proxy that list leaves out, every visitor shares the proxy's address and this one budget. |
| `ALPHONE_GRAPH_ANONYMOUS_CEILING` | How many graph requests callers who are not signed in run at once from every address together. Defaults to 20. |
| `ALPHONE_STREAMS_PER_USER` | How many subscriptions one caller holds open at once, and apart from them how many plugin requests. Defaults to 5 of each. |
| `ALPHONE_STREAM_LIFETIME` | How long one subscription or plugin request stays open. Defaults to `5m`. Each write of a subscription also gets this long to reach the caller. |

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

## Processors and memory

Each value is a whole number. Unset, AlphOne leaves both to Go, which runs
on as many processors as the container may use and sets no memory limit.
A value can only lower what Go would pick. A value AlphOne refuses stops
`serve` and `alphone check`.

| Variable | Purpose |
| --- | --- |
| `ALPHONE_MAX_PROCS` | The most processors that run AlphOne's code at once. It must stand at or below the processors of the machine. |
| `ALPHONE_MEMORY_LIMIT_MB` | The memory, in megabytes of 1048576 bytes, that AlphOne tries to stay under by freeing memory sooner. |

## Webhooks

AlphOne refuses to deliver a [webhook](/reference/webhooks/) to an
internal address unless you allow it here. Internal means loopback, the
private network ranges, the shared and reserved ranges, link-local and
multicast addresses, and their IPv6 counterparts. Public addresses are
reached on any port, apart from the cloud host addresses listed below.
A delivery also keeps its payload only for the time you set here.

| Variable | Purpose |
| --- | --- |
| `ALPHONE_WEBHOOK_ALLOWED_HOSTS` | The internal addresses webhooks may still reach, comma separated. Empty by default, which refuses them all. An entry is a CIDR range such as `127.0.0.1/32`, or a host name with a port such as `n8n:5678`. |
| `ALPHONE_WEBHOOK_PAYLOAD_AGE` | How long a delivery that was sent or failed for good keeps its payload, the event body it carried. Defaults to `720h`, 30 days. Then AlphOne clears the payload and keeps the rest of the delivery, such as its status and its last error. A delivery still waiting to be sent keeps its payload whatever its age. It must be a duration above zero, such as `168h`, and a value AlphOne refuses stops `serve` and the commands listed at the top of this page. |

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

`serve` will not start unless every entry is one of the two kinds, and
the commands listed at the top of this page stop too. A host name
without a port, such as `n8n`, stops them with:

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

- A browser that sends `Sec-Fetch-Site` says there where the page
  stood. A write from AlphOne's own pages passes, anything else is
  refused.
- A browser that does not send that header, such as Safari before
  version 16.4, is judged by its `Origin`. Its host and port must match
  the `Host` header of the write. The scheme is not compared.

That second way is why a reverse proxy in front of AlphOne should pass
the visitor's `Host` header through unchanged. AlphOne never reads
`X-Forwarded-Host` for this check. Caddy and Traefik keep the `Host`
header by default, and nginx needs `proxy_set_header Host $http_host`,
which keeps the port as well. Sending `Strict-Transport-Security` from
the proxy is optional. It keeps those browsers from loading one of
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

## Files

Plugins keep files, such as the attachments of a mail, in one place you
choose. They live in PostgreSQL by default, in the tables of the
`storage` schema, or in a bucket of an S3 compatible store.
[Where files live](/self-hosting/install/#where-files-live) helps you
choose, and [Backing up files](/self-hosting/updates-and-backups/#backing-up-files)
covers keeping them safe. The WhatsApp plugin keeps its media in a table
of its own whichever you choose.

| Variable | Purpose |
| --- | --- |
| `ALPHONE_STORAGE_DRIVER` | Where the files live, `postgres` or `s3`. Defaults to `postgres`. |
| `ALPHONE_STORAGE_MAX_BYTES` | The most bytes one file holds. Defaults to 26214400, which is 25 MiB. A larger file is refused. |
| `ALPHONE_STORAGE_CHUNK_BYTES` | With `postgres`, the most bytes of a file one database row holds. Defaults to 1048576, which is 1 MiB, and stops `serve` above 268435456, which is 256 MiB. A change applies to the files stored after it. |
| `ALPHONE_STORAGE_CLEANUP_TIMEOUT` | How long removing a failed upload may take: its rows with `postgres`, or the parts it already sent with `s3`. Defaults to `10s`. |
| `ALPHONE_STORAGE_SWEEP_INTERVAL` | With `postgres`, how often AlphOne deletes uploads that never finished, such as one a crash cut short. Defaults to `1h`. |
| `ALPHONE_STORAGE_SWEEP_AGE` | With `postgres`, how long an upload that never finished waits before it is deleted. Defaults to `24h`. |
| `ALPHONE_STORAGE_SWEEP_BATCH` | With `postgres`, the most uploads that never finished one sweep deletes. Defaults to 1000. |
| `ALPHONE_STORAGE_S3_ENDPOINT` | With `s3`, the address of the store, such as `https://s3.example.com`. It must be an `http` or `https` address. Unset, AlphOne uses the AWS address of the region. |
| `ALPHONE_STORAGE_S3_REGION` | With `s3`, the region AlphOne signs its requests for. Required. |
| `ALPHONE_STORAGE_S3_BUCKET` | With `s3`, the bucket the files live in. Required. Create it before you start AlphOne. |
| `ALPHONE_STORAGE_S3_ACCESS_KEY_ID` | With `s3`, the access key AlphOne signs with. Required. |
| `ALPHONE_STORAGE_S3_SECRET_ACCESS_KEY` | With `s3`, the secret of that access key. Required. |
| `ALPHONE_STORAGE_S3_PATH_STYLE` | With `s3`, `true` puts the bucket in the path of each request, as in `https://s3.example.com/bucket/...`, instead of in the host name. Defaults to `false`. Most stores other than AWS S3 want `true`. |
| `ALPHONE_STORAGE_S3_CHECKSUM` | With `s3`, `when_supported`, the default, adds a checksum to every request that takes one and checks every checksum an answer carries. `when_required` does both only where a request needs it, which some S3 compatible stores need, see [Where files live](/self-hosting/install/#where-files-live). |

Each byte size and count must be a whole number above zero, and each
time a duration above zero. An S3 setting with the `postgres` driver
stops `serve` with an error such as
`ALPHONE_STORAGE_S3_BUCKET is set but ALPHONE_STORAGE_DRIVER is not s3`.
With `s3`, a missing region, bucket or key stops it with an error such
as `ALPHONE_STORAGE_S3_REGION is required when ALPHONE_STORAGE_DRIVER is s3`.

Once the migrations ran, `serve` checks that it can reach the files and
stops when it cannot, with an error that names the driver, such as
`storage: the s3 bucket is not there, check the storage settings`.

## Encryption

Plugins keep some settings of each workspace as secrets, such as the
password of a mailbox, see [Settings](/extending/settings/). AlphOne
encrypts each secret with AES-256-GCM under the key you set here, and
ties it to its workspace and its setting, so a copy pasted anywhere else
does not open. The key lives only in the environment, never in the
database.

| Variable | Purpose |
| --- | --- |
| `ALPHONE_ENCRYPTION_KEY` | The key that encrypts every new secret, 32 bytes written as 64 hex characters. Make it once with `openssl rand -hex 32`. Unset, AlphOne still starts and keeps plain settings, and a plugin that saves or reads a secret gets an error that names this variable. |
| `ALPHONE_ENCRYPTION_PREVIOUS_KEYS` | The keys that still open the secrets saved before the current key, as a comma list of keys written the same way. Unset, only the current key opens them. It needs `ALPHONE_ENCRYPTION_KEY` set too. |

A key that is not hex or does not hold 32 bytes, or previous keys
without a current key, stops `serve` and the other commands listed at
the top of this page. The error names the variable and never shows the
key, such as `ALPHONE_ENCRYPTION_KEY: must hold 32 bytes, got 16`.

Keep a copy of every key off the server, see
[Backup scenario](/self-hosting/updates-and-backups/#backup-scenario).
Without the key no stored secret opens again.

### Replacing the key

1. Make a new key with `openssl rand -hex 32`.
2. Put the old key at the front of `ALPHONE_ENCRYPTION_PREVIOUS_KEYS`,
   and the new key in `ALPHONE_ENCRYPTION_KEY`.
3. Restart AlphOne. New secrets use the new key, and the older ones
   still open.
4. Keep every previous key. No command encrypts the older secrets again
   under the new key yet, so a key you drop takes the secrets it
   encrypted with it.

The order of the previous keys does not matter. Each encrypted value
carries an id that the key itself gives, and AlphOne tries every key
with that id.

## Email plugin

The email plugin stores the mail a workspace exchanges with its
contacts, see [Stored mail](/email/overview/). All of these are
optional. Each byte size and count must be a whole number above zero,
written without a unit, and each time a duration above zero, such as
`6h`. A value AlphOne refuses stops `serve` and the other commands
listed at the top of this page, with an error such as
`ALPHONE_EMAIL_SWEEP_BATCH: must stand above zero, got "0"`.

| Variable | Purpose |
| --- | --- |
| `ALPHONE_EMAIL_MESSAGE_MAX_BYTES` | The most bytes of one raw mail AlphOne reads whole. Defaults to 36700160, which is 35 MiB, so a mail carrying one attachment of 25 MiB still fits. A larger mail is read for its headers only and keeps no raw copy. A mail read whole keeps its raw copy as one file, so a raw copy over `ALPHONE_STORAGE_MAX_BYTES` is not kept, while the text and the attachments of that mail are. |
| `ALPHONE_EMAIL_MESSAGE_MAX_PARTS` | The most parts one mail holds. Defaults to 200. A mail with more is read for its headers only. |
| `ALPHONE_EMAIL_MESSAGE_MAX_CONTAINERS` | The most multipart containers one mail holds, the top one included. Defaults to 16. A mail with more is read for its headers only. Each level of containers a mail leaves open doubles the time it takes to read, so 16 levels take milliseconds and 24 take seconds. Keep this value low. |
| `ALPHONE_EMAIL_MESSAGE_MAX_FIELD_BYTES` | The most bytes of one `Content-Type` or `Content-Disposition` field. Defaults to 4096. A mail with a longer one is read for its headers only. |
| `ALPHONE_EMAIL_HEADER_MAX_BYTES` | The most bytes of the header block at the top of one mail. Defaults to 262144, which is 256 KiB. A mail with a larger one is read for the header fields that fit, and keeps its headers only. |
| `ALPHONE_EMAIL_BODY_MAX_BYTES` | The most bytes of text and HTML one mail keeps across all its parts. Defaults to 4194304, which is 4 MiB. A longer body is cut where a letter starts, and the raw copy keeps it whole when it fits. |
| `ALPHONE_EMAIL_SUBJECT_MAX_BYTES` | The most bytes of a subject that a mail and its thread keep, cut where a letter starts. Defaults to 2048. The event carries the subject as it is kept. |
| `ALPHONE_EMAIL_NAME_MAX_BYTES` | The most bytes of one display name, file name or content id a mail keeps, cut where a letter starts. Defaults to 512. |
| `ALPHONE_EMAIL_RECIPIENTS_MAX` | The most addresses of one mail that are matched to contacts and kept with it. Defaults to 500. Matching stops at the cap, so a contact named past it is not linked. The raw copy keeps every address. |
| `ALPHONE_EMAIL_MAILS_IN_MEMORY_MAX` | How many mails one AlphOne process reads and stores at once. Defaults to 1. A large mail can take a few hundred megabytes while it is read, so raise this only with memory to spare, and see [Processors and memory](#processors-and-memory) for the limit AlphOne tries to stay under. |
| `ALPHONE_EMAIL_SNIPPET_LETTERS` | The most letters of the short preview each mail keeps. Defaults to 200. An encrypted mail keeps no preview. |
| `ALPHONE_EMAIL_ATTACHMENT_MAX_BYTES` | The most bytes of one attachment or inline image kept as a file. Defaults to 26214400, which is 25 MiB. A larger one keeps its name with no file. A file over `ALPHONE_STORAGE_MAX_BYTES` keeps only its name too, whatever this value is. |
| `ALPHONE_EMAIL_WORKSPACE_MAX_BYTES` | The most bytes of mail each workspace keeps, which is the limit of every workspace. Defaults to 5368709120, which is 5 GiB. It counts the files of each mail, the text, HTML, preview and people each mail keeps, and the files still waiting for removal. A mail that would pass the limit keeps its text, and its files keep only their names. Mails stored at the same moment can pass it by up to their own size. `0` means no limit. It must be a whole number at or above zero. |
| `ALPHONE_EMAIL_EVENT_TEXT_MAX_BYTES` | The most bytes of a mail's text that the `email.message.received` event carries, cut where a letter starts. Defaults to 16384, which is 16 KiB. Each webhook delivery keeps a copy of it until `ALPHONE_WEBHOOK_PAYLOAD_AGE` after the delivery was sent or failed for good, see [Webhooks](#webhooks). |
| `ALPHONE_EMAIL_SUBJECT_THREAD_WINDOW` | How long after a thread's first or last mail a reply or a forward still joins it by its subject alone. A mail with no reply headers joins a thread by the provider's thread id only within this time too, see [Stored mail](/email/overview/#threads). Defaults to `336h`, 14 days. |
| `ALPHONE_EMAIL_THREAD_REFERENCES_MAX` | The most reply ids of one mail kept and used to find its thread, the first one and the newest ones. Defaults to 64. |
| `ALPHONE_EMAIL_RETRY_WINDOW` | How long a mail that keeps failing is tried again before it is set aside. Defaults to `6h`. AlphOne keeps only the reference and a fixed reason of a mail set aside, and answers `failed` for it from then on. A mail the database refuses for good is set aside at once. |
| `ALPHONE_EMAIL_CLEANUP_TIMEOUT` | How long queueing the files of a failed mail for removal may take, and how long recording its failure may take. Defaults to `10s`. The files are queued even when the run that failed was cancelled. A run that ran out of time still records its failure, and a run its caller cancelled records none, so its mail is tried again as if it never failed. The sweep removes the files it could not queue once `ALPHONE_EMAIL_ORPHAN_AGE` has passed. |
| `ALPHONE_EMAIL_SWEEP_INTERVAL` | How often AlphOne removes the files no stored mail holds, such as the files of a mail that failed to store. Defaults to `1h`. The first sweep runs one interval after `serve` starts. |
| `ALPHONE_EMAIL_SWEEP_BATCH` | The most files of one workspace a sweep takes in one step. Defaults to 100. A sweep takes a batch of each workspace in turn, so one workspace cannot hold up the others, and goes on until no file is left or its time runs out. |
| `ALPHONE_EMAIL_SWEEP_BUDGET` | How long one sweep may run. Defaults to `10m`. What is left waits for the next sweep. |
| `ALPHONE_EMAIL_ORPHAN_AGE` | How long a file that a stopped run left behind waits before the sweep removes it. Defaults to `24h`. Keep it longer than storing one mail can take. |

## Fields plugin

| Variable | Purpose |
| --- | --- |
| `ALPHONE_FIELDS_ENTRIES_MAX` | The most entries one repeater field holds on a contact. Defaults to 500. An add past the cap is refused. A value that is not a whole number between 1 and 2147483647 stops `serve` and the other commands listed at the top of this page. |

## WhatsApp plugin

All optional. With none of them and no number stored by a workspace, the
plugin runs inert: screens exist, but no webhook verifies and no message
sends. Values come from your Meta app, see
[Meta setup](/whatsapp/meta-setup/).

| Variable | Purpose |
| --- | --- |
| `ALPHONE_WHATSAPP_VERIFY_TOKEN` | The token Meta echoes during webhook verification. You invent it and paste the same value on both sides. |
| `ALPHONE_WHATSAPP_APP_SECRET` | The app secret, used to check the signature Meta sends with every webhook delivery. |
| `ALPHONE_WHATSAPP_ACCESS_TOKEN` | Bearer token for sending messages through the Graph API. |
| `ALPHONE_WHATSAPP_PHONE_NUMBER_ID` | The phone number ID (not the phone number itself) messages are sent from. |
| `ALPHONE_WHATSAPP_GRAPH_URL` | Graph API base URL. Defaults to `https://graph.facebook.com/v23.0`. Only override it for testing. |
| `ALPHONE_WHATSAPP_MEDIA_MAX_BYTES` | Largest inbound attachment stored, in bytes. Defaults to 26214400 (25 MiB), enough for every WhatsApp media type except large documents. Attachments over the cap appear in the thread as a named chip without a download. |
| `ALPHONE_WHATSAPP_DELIVERY_ITEMS_MAX` | The most messages and status updates stored from one webhook delivery. Defaults to 2000, above the 1000 updates Meta puts in one delivery. Anything past the cap is dropped and the delivery still answers `200`. A value that is not a whole number above zero stops `serve` and the other commands listed at the top of this page. |
| `ALPHONE_WHATSAPP_DELIVERY_NUMBERS_MAX` | The most numbers holding their own app secret that one webhook delivery may name. Defaults to 100, since Meta puts one business account in a delivery. A delivery past the cap proves nothing and answers `403`, even when `ALPHONE_WHATSAPP_APP_SECRET` signed it. A value that is not a whole number above zero stops `serve` and the other commands listed at the top of this page. |

A workspace can also connect any number of its own numbers, kept in the
database instead of in these settings. The access token and the app secret
of such a number are encrypted with `ALPHONE_ENCRYPTION_KEY`, see
[Encryption](#encryption). Without that key no such number can be stored.
A number connected without its own app secret or verify token uses
`ALPHONE_WHATSAPP_APP_SECRET` and `ALPHONE_WHATSAPP_VERIFY_TOKEN`. The number
in `ALPHONE_WHATSAPP_PHONE_NUMBER_ID` belongs to the default workspace, so no
other workspace can connect it. A conversation on that number keeps replying
from it when the default workspace connects other numbers. If the default
workspace connects that same number, the customer's next message opens a new
conversation, while the older one stays and replies from the same number.

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
