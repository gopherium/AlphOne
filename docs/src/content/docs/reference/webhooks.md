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

Five events are published. The list is deliberately short, because every
name is a public promise:

| Event | Published when |
| ----- | -------------- |
| `task.created` | A task is created, by a person or by an integration |
| `task.completed` | A task moves into `done` |
| `contact.created` | A contact is created, including one created by an inbound message from an unknown number |
| `whatsapp.message.received` | An inbound WhatsApp message is stored |
| `import.completed` | An import finishes turning its rows into contacts |

`task.completed` fires on the move into `done`, not on the state. Patching
a task that is already done publishes nothing, so an automation cannot
notify the same person twice.

## The envelope

Every delivery is a JSON body with the same four fields:

```json
{
  "id": "019fb38e-97d1-7f10-b3a4-52c30fe6a71b",
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

`id` names the event itself and stays the same across every retry of a
delivery, so store it and drop anything you have already processed.

`data` carries enough to identify the subject and read it at a glance.
For anything more, refetch through the API with `data.id`.

- `task.created` and `task.completed` carry `id`, `title`, `status`
- `due_on`, and `priority`. `contact.created` carries `id` and `name`.
- `whatsapp.message.received` carries `conversation_id`, `contact_id`,
`contact_name`, `external_id`, and `text`.
- `import.completed` carries `id`, `imported`, and `skipped`, so an automation 
can react to the contacts an import produced by refetching its rows.

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

AlphOne never follows a redirect. A `3xx` answer fails the attempt and
the delivery records `subscriber answered 301`, or whichever code came
back, so subscribe the final URL rather than one that forwards, such as
`https://` rather than `http://`.

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

Subscriptions are managed on the graph, with `createWebhook`, `webhooks` and
`deleteWebhook`. Every operation needs a credential, see
[authenticating](/reference/graphql-api/#authenticating).

Creating a subscription needs the `manage_webhooks` capability, which only the
admin role holds in a stock install, see [roles](/reference/graphql-api/#roles).
A subscription belongs to the account that created it. An account holding
`manage_webhooks` sees and revokes every subscription of its workspace. Any
other account sees and revokes only its own.

A subscription keeps firing whatever its owner's role. One created by an
account that does not hold `manage_webhooks`, such as a member's from an
earlier release, still delivers, and an account holding the capability sees it
and can revoke it.

Disabling the owner's account stops its subscriptions. Events published while
it is disabled are not delivered to them, and enabling the account again
resumes them. A delivery queued earlier that comes due while the account is
disabled is dropped instead of sent. It fails with `webhook: owner disabled` in
its `last_error`, and enabling the account again does not bring it back.

### Creating one

```graphql
mutation {
  createWebhook(
    url: "https://example.com/hooks/alphone"
    events: ["task.created", "contact.created"]
  ) {
    webhook {
      id
      url
      events
      createdAt
    }
    secret
  }
}
```

The `secret` appears here and nowhere else. Store it now. To replace a
lost one, revoke the subscription and create another.

An account without `manage_webhooks` is refused with the message
`admin required`, the code `UNAUTHORIZED` and the reason `capability_missing`.
The error's `scope` extension names `webhooks:write` and its `capability`
extension names `manage_webhooks`. A token also needs the `webhooks:write`
scope, and one without it is refused with `scope required: webhooks:write`.

An unusable URL or an event name AlphOne does not publish is refused with the
code `VALIDATION`.

A URL written as an internal IP address, such as `http://127.0.0.1:5678/` or
`http://10.0.0.5/`, is refused with the code `VALIDATION` and the reason
`webhook_url_internal`, unless the operator allowed it, see
[internal addresses](#internal-addresses). A host name is not looked up at this
point, so a name that points to an internal address is accepted and its
deliveries are refused later.

### Listing them

```graphql
query {
  webhooks {
    id
    url
    events
    createdAt
    owner {
      id
      name
      email
    }
  }
}
```

An account holding `manage_webhooks` lists every subscription of its
workspace, newest first. Any other account lists only its own. `owner` names
the account that created each one, and is null once that account no longer
exists.

Secrets are never listed.

### Revoking one

```graphql
mutation ($id: UUID!) {
  deleteWebhook(id: $id)
}
```

Revoking answers `true` and drops any deliveries still queued for the
subscription. An account holding `manage_webhooks` revokes any subscription of
its workspace. Any other account that tries to revoke one it did not create is
refused with the code `NOT_FOUND` and the reason `webhook_not_found`, the same
as one that never existed.

## Notes for self-hosters

The signing secret is stored so AlphOne can read it, because signing a
delivery requires the secret itself. That differs from API tokens and
passwords, which are stored hashed. Treat database backups accordingly.

### Internal addresses

AlphOne refuses to deliver to internal addresses: loopback, the private
network ranges, link-local and cloud metadata addresses, and their IPv6
counterparts. Public addresses are reached on any port, apart from a
few cloud metadata addresses, see
[Configuration](/self-hosting/configuration/#webhooks).

The check runs on the address AlphOne is about to connect to, after the name
lookup, so every attempt is checked again. A refused delivery counts as a
failed attempt and is retried like any other. Its `last_error` names
`webhook: address refused`, and the log gets a warning,
`refusing a webhook delivery to an internal address`, naming the host.

To deliver to a service on your own network, such as n8n on the same Docker
network or on the same machine, list it in `ALPHONE_WEBHOOK_ALLOWED_HOSTS`,
for example `n8n:5678` or `localhost:5678`. A URL that names no port is
listed with 80 for `http` or 443 for `https`. Link-local and cloud metadata
addresses stay refused even when listed. See
[Configuration](/self-hosting/configuration/#webhooks) for the format.

Deliveries ignore `HTTP_PROXY` and `HTTPS_PROXY` and always connect to the
subscriber directly.
