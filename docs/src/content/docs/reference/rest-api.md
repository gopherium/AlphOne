---
title: REST API
description: The JSON API behind the AlphOne frontend. Endpoint reference in progress.
---

:::note[In progress]
The full endpoint reference is being written. The basics below are
enough to explore the API today.
:::

Everything the frontend does goes through the JSON API under `/api`.
There is no separate integration surface: what the UI can do, the API
can do.

## Authentication

The API uses cookie sessions, not API keys. Obtain a session by posting
credentials, then send its cookie with every request:

```sh
curl -c cookies.txt -H 'Content-Type: application/json' \
  -d '{"email":"you@example.com","password":"..."}' \
  https://your-domain/api/auth/login

curl -b cookies.txt https://your-domain/api/auth/session
```

`POST /api/auth/logout` ends the session. Failed logins are rate-limited
per client address, 10 per minute, answered with `429` and a
`Retry-After` header when exceeded.

## Shape of the API

- Requests and responses are JSON. Errors are
  `{"error": "<message>"}` with a matching HTTP status.
- Every route except login, logout, and plugin webhooks requires a
  session.
- Plugin endpoints live under `/api/plugins/<id>/`, for example
  `/api/plugins/whatsapp/conversations`.
- `GET /api/version` reports the running version.

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
`priority` is an integer from `0` to `9`, where `0` is normal. The
`origin_*` pair records the event a task was created from and is
reserved for later use.

### List tasks

`GET /api/tasks` takes exactly one of these filters, and answers `400`
otherwise:

| Filter | Returns |
| --- | --- |
| `date=YYYY-MM-DD` | your tasks due that day |
| `due_before=YYYY-MM-DD` | your open tasks due earlier, the overdue list |
| `contact_id=<uuid>` | a contact's tasks, across every assignee |

Add `status=open`, `status=done`, or `status=all`. The default is `open`.

Results are ordered by due date, then id. Pages carry at most `limit`
tasks, 50 by default and 200 at most:

```json
{ "tasks": [], "next_cursor": null }
```

`next_cursor` is an opaque string. Pass it back as `cursor` for the next
page, and stop when it is `null`.

### Read, create, and update

`GET /api/tasks/{id}` returns one task.

`POST /api/tasks` creates one. `title` and `due_on` are required,
`priority` and `contact_id` optional. The task is always assigned to the
session user: an `assignee_id` in the body is ignored.

```sh
curl -b cookies.txt -H 'Content-Type: application/json' \
  -d '{"title":"Call the supplier","due_on":"2026-07-30"}' \
  https://your-domain/api/tasks
```

`PATCH /api/tasks/{id}` updates `title`, `due_on`, `status`, or
`priority`. Any field you leave out stays as it is, so completing a task
is one field:

```sh
curl -b cookies.txt -X PATCH -H 'Content-Type: application/json' \
  -d '{"status":"done"}' \
  https://your-domain/api/tasks/0198d000-0000-7000-8000-000000000001
```

Invalid values answer `422`, an unknown id answers `404`, and malformed
ids, dates, or query parameters answer `400`. Tasks cannot be deleted.
