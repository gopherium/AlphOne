---
title: REST API
description: The two routes AlphOne still serves outside the graph.
---

The [GraphQL API](/reference/graphql-api/) is AlphOne's API. Every read and
every write is a `POST /api/graphql`.

The REST routes that used to answer beside it were removed in 0.8.0. If you
built against them, the 0.8.0 release notes map each one to the graph
operation that replaced it.

## Staying on REST

Two routes are not going anywhere, because neither is something a GraphQL
client asks for.

**Meta's webhook pair.** `GET` and `POST /api/plugins/whatsapp/webhook`. Meta
calls these, so their shape is Meta's to decide, not ours. They authenticate by
signature rather than by session.

**The media download.** `GET
/api/plugins/whatsapp/conversations/{id}/messages/{mid}/media` answers with the
file bytes. A graph field can name a download path, but the bytes themselves
need a plain HTTP response.

Both are documented in the [WhatsApp API](/whatsapp/api/).

## Limits on what remains

A plugin route that needs a session carries two bounds the graph does not. One
user may have 5 such requests open at once, and any one of them is closed after
5 minutes. Over the cap the answer is `429` with a `Retry-After` header.

Paths a plugin declares public, such as the webhook pair, skip both.

## Live events

Live updates left REST at 0.7.0. The graph serves them as subscriptions over
Server-Sent Events, so a client posts to `/api/graphql` with
`Accept: text/event-stream`. See
[subscriptions](/reference/graphql-api/#subscriptions).

For payloads and reliable delivery, subscribe a
[webhook](/reference/webhooks/) instead. The delivery contract did not move.
