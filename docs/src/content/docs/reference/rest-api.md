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
