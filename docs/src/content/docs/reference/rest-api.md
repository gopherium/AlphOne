---
title: REST API
description: The JSON API behind the AlphOne frontend, endpoint by endpoint.
---

Everything the frontend does goes through the JSON API under `/api`.
There is no separate integration surface: what the UI can do, the API
can do.

## Conventions

**Authentication.** Every route needs a credential except `POST
/api/auth/login`, `POST /api/auth/logout`, and the public paths a plugin
declares, such as the WhatsApp webhook. Requests without a usable
credential get `401`.

Two credentials are accepted. Browsers use a session cookie named
`__Host-alphone_session`, set with `HttpOnly`, `Secure`, `SameSite=Lax`,
and a 30 day lifetime. Programs use an API token in an `Authorization`
header:

```http
GET /api/tasks HTTP/1.1
Authorization: Bearer a1_xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx
```

A token acts as the user who created it and carries the same permissions.
Tokens never expire, so revoke one you no longer need. Disabling a user
stops their tokens with their session. A request presenting an
unrecognised token gets `401`, and a request whose `Authorization` header
is not a bearer credential falls back to the session cookie.

Mint tokens with the `token` subcommand. The secret is shown once and
stored only as a hash, so a lost secret cannot be recovered, only
replaced:

```sh
alphone token create -email you@example.com -name "n8n production"
alphone token list   -email you@example.com
alphone token revoke -email you@example.com -id <token id>
```

**Bodies.** Requests and responses are JSON. Request bodies are capped
at 1 MiB, and anything unparseable, empty, or oversized answers `400`.
Unknown fields in a request body are ignored rather than rejected. A
known field your credential may not set is rejected, as with
`origin_event_id` on a session request.

**Errors.** Every error carries the same envelope:

```json
{ "error": "invalid limit" }
```

The message is written for people and can change between versions.
Branch on the status code, not on the text. `204` responses carry no
body at all.

**Partial updates.** `PATCH` bodies name only the fields you want to
change. A field you leave out keeps its current value, and so does a
field sent as `null`. A `PATCH` that changes nothing answers `200` with
the resource untouched.

**Pagination.** `GET /api/contacts` and `GET /api/tasks` page the same
way. `limit` defaults to 50 and accepts 1 to 200. `cursor` is an opaque
string copied from the previous response. Treat it as opaque, do not
build one by hand.

```json
{ "contacts": [], "next_cursor": null }
```

The list key is always an array, never `null`, and `next_cursor` is
always present. A `null` cursor means there is no next page, and that is
the only end-of-list signal: there is no total count.

**Timestamps.** `created_at` and friends are RFC 3339 in UTC. A task's
`due_on` is a plain calendar date, `YYYY-MM-DD`.

## Authentication

`POST /api/auth/login` takes `email` and `password`, sets the session
cookie, and returns the signed-in identity:

```sh
curl -c cookies.txt -H 'Content-Type: application/json' \
  -d '{"email":"you@example.com","password":"..."}' \
  https://your-domain/api/auth/login
```

```json
{ "id": "0198...", "email": "you@example.com", "name": "Your Name" }
```

An unknown email, a wrong password, and a disabled account all answer
`401` with the same message, so the API never reveals which accounts
exist. Failed logins are limited to 10 per minute per client address.
Once over the limit the answer is `429` with a `Retry-After` header, and
successful logins never count against it.

`GET /api/auth/session` returns the same identity object, and is how the
frontend checks whether a session is still alive.

`POST /api/auth/logout` clears the cookie and answers `204`, including
when no session was sent.

:::caution
Behind a reverse proxy, set `ALPHONE_TRUSTED_PROXIES` to the proxy's
network. Without it the rate limiter cannot see client addresses and
every user shares one budget.
:::

## Contacts

A contact is a person, with per-channel identities attached to them by
plugins.

```json
{
  "id": "0198c000-0000-7000-8000-000000000001",
  "name": "Maria Perez",
  "created_at": "2026-07-29T10:00:00Z"
}
```

`GET /api/contacts` lists them in name order. Add `q` to search, which
matches the contact's name, an identity's display name, and phone-style
identifiers. Digits are pulled out of the query for the identifier
match, so `q=+1 844 672` finds the identity stored as `184467235`.

`GET /api/contacts/{id}` returns one contact with its identities:

```json
{
  "id": "0198c000-0000-7000-8000-000000000001",
  "name": "Maria Perez",
  "created_at": "2026-07-29T10:00:00Z",
  "identities": [
    {
      "id": "0198c000-0000-7000-8000-000000000002",
      "channel": "whatsapp",
      "identifier": "184467235",
      "display_name": "Maria"
    }
  ]
}
```

`identities` is always an array, and empty for a contact no channel has
reached yet. The API writes the `email` and `phone` channels. Every
other channel is created by its plugin, `whatsapp` identities for
example arrive with inbound messages.

Identifiers are normalized before storage: emails are lowercased and
phones keep only their digits and a leading plus, so `+1 (844) 672-35`
is stored as `+184467235`. An identifier is unique per channel across
all contacts.

`POST /api/contacts/{id}/identities` takes
`{"channel": "email", "identifier": "maria@example.com", "display_name": "Work"}`
and answers `201` with the created identity. `display_name` is
optional. A channel other than `email` or `phone` answers `422`. When
another contact already holds the identifier, the answer is `409` and
names the owner:

```json
{
  "error": "contact: identity already exists",
  "owner": {
    "id": "0198c000-0000-7000-8000-000000000001",
    "name": "Maria Perez",
    "created_at": "2026-07-29T10:00:00Z"
  }
}
```

`DELETE /api/contacts/{id}/identities/{identityId}` answers `204`, or
`404` when the identity does not exist under that contact.

`POST /api/contacts` takes `{"name": "..."}` and answers `201` with the
created contact. Names are not unique. An optional `identities` array
in the same shape creates the contact and its identities together, all
or nothing, with the same `422` and `409` answers.

`PATCH /api/contacts/{id}` takes `{"name": "..."}` and answers `200`
with the updated contact. A blank name answers `422`. Contacts cannot be
deleted.

## Tasks

A task is a piece of work owned by one user, due on one calendar day,
optionally linked to a contact.

```json
{
  "id": "0198d000-0000-7000-8000-000000000001",
  "assignee_id": "0198c000-0000-7000-8000-0000000000aa",
  "contact_id": null,
  "title": "Call the supplier",
  "status": "open",
  "priority": 0,
  "due_on": "2026-07-30",
  "origin_source": null,
  "origin_event_id": null,
  "created_at": "2026-07-29T10:00:00Z"
}
```

`due_on` is a calendar date, not a timestamp, so rescheduling is day
arithmetic in the caller's own timezone. `status` is `open` or `done`.
`priority` is an integer from `0` to `9`, where `0` is normal.
`contact_id`, `origin_source`, and `origin_event_id` are always present
and `null` when unset. `origin_source` records how a task came to exist:
the server stamps `token:<name>` on tasks created with an API token and
leaves it `null` for a browser session. `origin_event_id` records the
event an automation was reacting to, and it is what makes a repeated
create safe. See [idempotent creation](#idempotent-creation).

### List tasks

`GET /api/tasks` takes exactly one of these filters, and answers `400`
otherwise:

| Filter | Returns |
| --- | --- |
| `date=YYYY-MM-DD` | your tasks due that day |
| `due_before=YYYY-MM-DD` | your open tasks due earlier, the overdue list |
| `contact_id=<uuid>` | a contact's tasks, across every assignee |

Add `status=open`, `status=done`, or `status=all`. The default is
`open`. Results are ordered by due date, then id.

The `date` and `due_before` filters answer for the signed-in user only.
The `contact_id` filter deliberately spans every assignee, because a
contact's page is shared context.

### Read, create, and update

`GET /api/tasks/{id}` returns one task.

`POST /api/tasks` creates one. `title` and `due_on` are required,
`priority` and `contact_id` optional, and `origin_event_id` optional for
an API token as described below. The task is always assigned to the
session user: an `assignee_id` in the body is ignored.

```sh
curl -b cookies.txt -H 'Content-Type: application/json' \
  -d '{"title":"Call the supplier","due_on":"2026-07-30"}' \
  https://your-domain/api/tasks
```

### Idempotent creation

Delivery to an automation is at least once, so the same event can reach
your workflow twice. To make a repeated create harmless, send the event
you are reacting to as `origin_event_id`:

```sh
curl -H 'Authorization: Bearer a1_xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx' \
  -H 'Content-Type: application/json' \
  -d '{"title":"Call the supplier","due_on":"2026-07-30",
       "origin_event_id":"0198d000-0000-7000-8000-0000000000e1"}' \
  https://your-domain/api/tasks
```

The first call answers `201` with the new task. Any later call carrying
the same `origin_event_id` answers `200` with the task already created,
and creates nothing. The rest of the repeat's body is ignored, so use
`PATCH` to change a task you already created.

Any uuid your workflow can reproduce for the same piece of work will do.
The envelope `id` of a [webhook](/reference/webhooks/) event is the usual
choice, because it names the event and stays the same across every
delivery and every retry. Do not use the `X-AlphOne-Delivery` header,
which is a per attempt id and differs for every subscription.

Only an API token may send `origin_event_id`, since the key is scoped by
the token's name and by the user the token acts as. A browser session
sending it answers `422`. Two tokens of one user sharing a name share one
key space, so give tokens distinct names.

`PATCH /api/tasks/{id}` updates `title`, `due_on`, `status`, or
`priority`. Completing a task is one field:

```sh
curl -b cookies.txt -X PATCH -H 'Content-Type: application/json' \
  -d '{"status":"done"}' \
  https://your-domain/api/tasks/0198d000-0000-7000-8000-000000000001
```

Invalid values answer `422`, an unknown id answers `404`, and malformed
ids, dates, or query parameters answer `400`. Tasks cannot be deleted.

## Users

```json
{
  "id": "0198b000-0000-7000-8000-000000000001",
  "email": "you@example.com",
  "name": "Your Name",
  "disabled": false,
  "created_at": "2026-07-29T10:00:00Z"
}
```

`GET /api/users` returns every account as a plain array. There is no
pagination here, and password hashes are never included.

`POST /api/users` takes `email`, `name`, and `password`, and answers
`201`. Passwords must be at least 12 characters. An address already in
use answers `409`, and anything else invalid answers `422` naming the
first problem found.

`PATCH /api/users/{id}` takes `{"disabled": true}` or `false` and
answers `204` with no body. Disabling an account also ends its live
sessions on the next request. You cannot disable your own account.

:::caution
AlphOne has no roles yet. Every signed-in user can list, create, and
disable accounts. Treat every account as an administrator until roles
land.
:::

## Webhooks

`GET /api/webhooks`, `POST /api/webhooks`, and `DELETE
/api/webhooks/{id}` manage the endpoints AlphOne posts events to. The
[webhooks reference](/reference/webhooks/) covers the envelope, the
signature, and the retry policy.

## Live events

Live updates no longer ride a REST route. The GraphQL endpoint serves
them as subscriptions over Server-Sent Events, so a client posts to
`/api/graphql` with `Accept: text/event-stream`.

```graphql
subscription {
  coreEvent
}
```

`coreEvent` announces published event names, `task.created` and the
rest, names only. Shared events reach every open session, whoever caused
the change. Task events reach only the assignee, so a session hears
nothing when another user's task is created or completed. For payloads
or reliable delivery, subscribe a [webhook](/reference/webhooks/)
instead: the subscription replays nothing and drops names a slow reader
misses. Subscriptions rotate on a lifetime bound and are capped per
user, so treat a close as routine and reconnect.

## Version

`GET /api/version` returns the running release as `{"version": "x.y.z"}`.
It needs a session, so it is not a public health probe.

## Plugins

Plugin endpoints live under `/api/plugins/<id>/`, for example
`/api/plugins/whatsapp/conversations`. See the
[WhatsApp API](/whatsapp/api/) for the endpoints that plugin serves.

Plugin routes carry two limits the core routes do not. One user may have
5 session-protected plugin requests open at once, and any one of them is
closed after 5 minutes. Both matter for event streams: a stream holds a
slot for its whole life, and a client must reconnect when the cap
expires. Over the limit the answer is `429`.

A plugin may declare paths that need no session, such as a webhook that
authenticates itself by signature. Those paths skip both limits as well.
