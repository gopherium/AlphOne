---
title: Webhooks
description: Receive signed AlphOne events over HTTP.
---

AlphOne posts an event to your endpoint when something happens. Subscribe
an URL, verify the signature, and act.

Webhooks are how an automation engine reacts to AlphOne without polling.
Nothing here is specific to any engine, see the
[automation guide](/guides/automation/) for pairing one up.

## Events

Four events are published. The list is deliberately short, because every
name is a public promise:

| Event | Published when |
| ----- | -------------- |
| `task.created` | A task is created, by a person or by an integration |
| `task.completed` | A task moves into `done` |
| `contact.created` | A contact is created, including one created by an inbound message from an unknown number |
| `whatsapp.message.received` | An inbound WhatsApp message is stored |

`task.completed` fires on the move into `done`, not on the state. Patching
a task that is already done publishes nothing, so an automation cannot
notify the same person twice.

## The envelope

Every delivery is a JSON body with the same three fields:

```json
{
  "event": "task.created",
  "occurred_at": "2026-07-30T15:04:54.498347434Z",
  "data": {
    "id": "019fb38e-97e0-75cc-bab1-680b41e86588",
    "title": "Call Maria about the renewal",
    "status": "open",
    "due_on": "2026-08-01",
    "priority": 0
  }
}
```

`data` carries enough to identify the subject and read it at a glance.
For anything more, refetch through the API with the `id`.

`task.created` and `task.completed` carry `id`, `title`, `status`,
`due_on`, and `priority`. `contact.created` carries `id` and `name`.
`whatsapp.message.received` carries `conversation_id`, `contact_id`,
`contact_name`, `external_id`, and `text`.

## Headers

| Header | Value |
| ------ | ----- |
| `Content-Type` | `application/json` |
| `X-AlphOne-Event` | The event name, so you can route without parsing the body |
| `X-AlphOne-Delivery` | A unique id for this attempt, useful in your logs |
| `X-AlphOne-Signature-256` | `sha256=` followed by the signature in hex |
| `User-Agent` | `AlphOne-Webhook/1` |

## Verifying the signature

The signature is an HMAC-SHA256 of the **raw request body** using the
secret you received when you subscribed. Compute it over the bytes you
received, before any JSON parsing, and compare in constant time.

```python
import hmac, hashlib

def verify(secret: str, body: bytes, header: str) -> bool:
    want = "sha256=" + hmac.new(secret.encode(), body, hashlib.sha256).hexdigest()
    return hmac.compare_digest(header, want)
```

```js
import { createHmac, timingSafeEqual } from 'node:crypto'

function verify(secret, body, header) {
	const want = 'sha256=' + createHmac('sha256', secret).update(body).digest('hex')
	return header.length === want.length && timingSafeEqual(Buffer.from(header), Buffer.from(want))
}
```

Reject anything that does not verify. Without this check, anyone who
learns your endpoint can invent events.

## Retries

Answer with any `2xx` to accept a delivery. Anything else, including a
redirect, a timeout, or a refused connection, counts as a failure and is
retried.

A failed delivery is retried with a wait that doubles each time, starting
at 30 seconds and capped at one hour:

| Attempt | Waits before it |
| ------- | --------------- |
| 1 | delivered as soon as the event happens |
| 2 | 30 seconds |
| 3 | 1 minute |
| 4 | 2 minutes |
| onwards | doubling up to 1 hour, then hourly |

Retries continue for at least 24 hours after the event. Then the delivery
is marked failed and never retried, so an endpoint that stays down for
more than a day misses the event. AlphOne waits 10 seconds for a
response, so answer quickly and do the work afterwards.

Deliveries are queued in the database, so a restart mid delivery does not
lose them.

## Managing subscriptions

Every route needs a credential, see
[authentication](/reference/rest-api/). A subscription belongs to the user
who created it, who is the only one who can see or revoke it.

### `POST /api/webhooks`

```json
{
  "url": "https://example.com/hooks/alphone",
  "events": ["task.created", "contact.created"]
}
```

Answers `201` with the subscription and its signing secret:

```json
{
  "id": "019fb341-270d-744e-b0fd-8d18d53cd6be",
  "url": "https://example.com/hooks/alphone",
  "events": ["task.created", "contact.created"],
  "secret": "whsec_GlNGnDYSNuOdBCKSaglvqmUDUVONRtNtCmTk4OOH1WQ",
  "created_at": "2026-07-30T13:26:47.720971759Z"
}
```

The `secret` appears here and nowhere else. Store it now. To replace a
lost one, revoke the subscription and create another.

An unusable URL or an event name AlphOne does not publish answers `422`.

### `GET /api/webhooks`

```json
{
  "webhooks": [
    {
      "id": "019fb341-270d-744e-b0fd-8d18d53cd6be",
      "url": "https://example.com/hooks/alphone",
      "events": ["task.created", "contact.created"],
      "created_at": "2026-07-30T13:26:47.720971759Z"
    }
  ]
}
```

Secrets are never listed.

### `DELETE /api/webhooks/{id}`

Answers `204`, and drops any deliveries still queued for it. Revoking
someone else's subscription answers `404`, the same as one that never
existed.

## Notes for self-hosters

The signing secret is stored so AlphOne can read it, because signing a
delivery requires the secret itself. That differs from API tokens and
passwords, which are stored hashed. Treat database backups accordingly.

Any URL is accepted, including private addresses on your own network,
which is what makes an engine on the same host reachable. AlphOne trusts
the operator here.
