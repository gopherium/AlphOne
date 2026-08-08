---
title: WhatsApp API
description: The endpoints the WhatsApp plugin serves, from the Meta webhook to conversations, messages, media, and the live stream.
---

The WhatsApp plugin serves its own endpoints under
`/api/plugins/whatsapp/`. They follow the
[core API conventions](/reference/rest-api/): a session cookie, JSON
bodies, and the same error envelope. The webhook is the exception, and
authenticates itself with a signature instead.

## Webhook

`GET /api/plugins/whatsapp/webhook` answers Meta's verification
handshake. It compares `hub.verify_token` against
`ALPHONE_WHATSAPP_VERIFY_TOKEN` and echoes `hub.challenge` back as plain
text. A missing token setting, a wrong token, or a `hub.mode` other than
`subscribe` answers `403`.

`POST /api/plugins/whatsapp/webhook` receives message and status
events. Every request must carry `X-Hub-Signature-256`, the value being
`sha256=` followed by the hex HMAC-SHA256 of the raw body keyed with
`ALPHONE_WHATSAPP_APP_SECRET`. A missing, malformed, or wrong signature
answers `403`, and so does an unset app secret. The webhook never
falls open.

The reply is a bare status with no body: `200` once the payload has been
stored, `400` for an unreadable payload, and `500` when storing fails so
that Meta retries. Redelivered messages are recognised and stored once.

Both routes are reachable without a session, which is why the signature
check matters. Everything else on this page needs one.

## Conversations

`GET /api/plugins/whatsapp/conversations` returns the threads in
most-recent-activity order as a plain array. `limit` defaults to 50 and
accepts 1 to 200. There is no cursor here.

```json
[
  {
    "id": "0198e000-0000-7000-8000-000000000001",
    "contact_id": "0198c000-0000-7000-8000-000000000001",
    "contact_name": "Maria Perez",
    "external_id": "184467235",
    "status": "open",
    "last_activity_at": "2026-07-29T10:00:00Z",
    "last_message_preview": "Is it this model?"
  }
]
```

`contact_name` is read live from the contact, so renaming a contact
changes every thread that belongs to them. `last_message_preview` is
`null` only for a thread with no messages.

## Messages

`GET /api/plugins/whatsapp/conversations/{id}/messages` returns a
thread in send order, oldest first, with the same `limit` rules.

```json
[
  {
    "id": "0198e000-0000-7000-8000-000000000002",
    "external_id": "wamid.HBg...",
    "direction": "inbound",
    "content": "Is it this model?",
    "content_type": "image",
    "sent_at": "2026-07-29T10:00:00Z",
    "status": null,
    "status_detail": null,
    "media": {
      "status": "stored",
      "mime_type": "image/jpeg",
      "filename": null,
      "file_size": 51234,
      "voice": false,
      "animated": false
    }
  }
]
```

`status` and `status_detail` describe delivery and are set on outbound
messages only, moving through `sent`, `delivered`, `read`, and `played`,
or `failed` with a reason in `status_detail`. Both are `null` on inbound
messages and on an outbound message no status has arrived for yet.

`media` is `null` for text. When present, its own `status` is `pending`
while the file is still being fetched from Meta, then `stored`, or
`failed` when the file could not be retrieved.

`POST /api/plugins/whatsapp/conversations/{id}/messages` sends a text
reply. The body is `{"content": "..."}` and nothing else is read, so
media and templates cannot be sent through it yet.

```sh
curl -b cookies.txt -H 'Content-Type: application/json' \
  -d '{"content":"Ready at 5pm, see you there."}' \
  https://your-domain/api/plugins/whatsapp/conversations/<id>/messages
```

The reply is `201` with the stored message, whose `status` is `null`
until Meta reports one. A blank message answers `400`, an unknown
conversation `404`, and a rejection from Meta answers `502` carrying
their own message and code:

```json
{
  "error": "Message failed to send because more than 24 hours have passed since the customer last replied",
  "code": 131047
}
```

## Media

`GET /api/plugins/whatsapp/conversations/{id}/messages/{mid}/media`
serves a stored file. The response carries the stored content type,
`Cache-Control: private, max-age=31536000, immutable`, an `ETag`, and a
sandboxing `Content-Security-Policy`. Range requests and conditional
requests are supported, so browsers can seek audio and video and skip
re-downloads.

A message with no media, or whose media is still pending or failed,
answers `404`.

## Live updates

Thread changes ride the GraphQL subscriptions on `/api/graphql` rather
than a REST route. A client posts with `Accept: text/event-stream`.

```graphql
subscription {
  whatsAppConversationEvent
}
```

`whatsAppConversationEvent` names the thread that changed, whether from
an inbound message, a delivery status, or a downloaded file, so a client
refetches the parts it cares about. There is no replay after a
disconnect.

An open thread can take its arrivals directly instead of refetching.

```graphql
subscription ($id: UUID!) {
  whatsAppMessageReceived(conversationId: $id) {
    id
    content
    sentAt
  }
}
```

A subscription holds one of the 5 concurrent stream slots a user has,
and is closed after 5 minutes. Clients are expected to reconnect.
