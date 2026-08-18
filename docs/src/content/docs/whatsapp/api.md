---
title: WhatsApp API
description: The two endpoints the WhatsApp plugin serves outside the graph, the Meta webhook and the media download, beside its live stream.
---

Conversations, messages and replies are graph operations. Read them as
`whatsAppConversations`, `whatsAppConversation.messages` and
`whatsAppSendMessage` in the [GraphQL API](/reference/graphql-api/).

Two endpoints stay outside the graph, because neither is something a GraphQL
client asks for. They live under `/api/plugins/whatsapp/`. The media download
takes a session cookie or a bearer token, see
[authenticating](/reference/graphql-api/#authenticating). A bearer token needs
the `whatsapp` area, so mint it with `-scope whatsapp:read`. The webhook
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
check matters. The media download needs one.

## Media

`GET /api/plugins/whatsapp/conversations/{id}/messages/{mid}/media`
serves a stored file. A graph field can name a download path, but the bytes
themselves need a plain HTTP response, so this route stays. The
`whatsAppMessage.media.downloadPath` field gives you the address to call.

The response carries the stored content type,
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
