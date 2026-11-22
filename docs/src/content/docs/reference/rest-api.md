---
title: REST API
description: The routes AlphOne serves outside the graph.
---

The [GraphQL API](/reference/graphql-api/) is AlphOne's API. Every read and
every write is a `POST /api/graphql`.

The REST routes that used to answer beside it were removed in 0.8.0. If you
built against them, the 0.8.0 release notes map each one to the graph
operation that replaced it.

## Staying outside the graph

Five routes are not going anywhere, because none of them is something a
GraphQL client asks for.

**Meta's webhook pair.** `GET` and `POST /api/plugins/whatsapp/webhook`. Meta
calls these, so their shape is Meta's to decide, not ours. They authenticate by
signature rather than by session.

**The media download.** `GET
/api/plugins/whatsapp/conversations/{id}/messages/{mid}/media` answers with the
file bytes. A graph field can name a download path, but the bytes themselves
need a plain HTTP response.

**The email downloads.** `GET /api/plugins/email/attachments/{id}` answers one
attachment of a stored mail. `GET /api/plugins/email/messages/{id}/original`
answers the whole original mail as `message/rfc822`, saved as
`original-<id>.eml`. The graph names both paths, in `downloadPath` and
`originalPath`. Every file comes with `Content-Security-Policy: sandbox`,
`X-Content-Type-Options: nosniff` and `Cache-Control: private, no-store`, so
it never runs as a page and no cache keeps it. The saved name is always safe.

**The MCP endpoint.** `POST /api/mcp` speaks the Model Context Protocol
(MCP), so an AI agent can read AlphOne with its own client. It takes the same
API tokens, and its tools run graph operations underneath. See
[AI agents](/guides/agents/).

The webhook pair and the media download are documented in the
[WhatsApp API](/whatsapp/api/). The email downloads are documented in
[Stored mail](/email/overview/#downloading-files).

## Limits on what remains

A plugin route that needs a session carries two bounds the graph does not. One
user may have 5 such requests open at once, and any one of them is closed after
5 minutes. Over the cap the answer is `429` with a `Retry-After` header.

Paths a plugin declares public, such as the webhook pair, skip both.

A plugin may also hold its routes to one scope area, and WhatsApp and email
do. The media download needs `whatsapp:read`, so a token scoped elsewhere is
rejected with `403`:

```json
{ "error": "scope required: whatsapp:read" }
```

The email downloads need `email:read`, and a token without it gets:

```json
{ "error": "scope required: email:read" }
```

The check runs before the plugin sees the request, so an under-scoped token
gets that same `403` for a path the plugin does not serve at all, where a
token holding the area would get `404`. A session is not narrowed this way,
because only a token carries scopes.

A plugin route that panics answers `500` with `Cache-Control: no-store`, on
public paths too. The body is AlphOne's internal error and never names the
plugin:

```json
{ "error": "internal error", "code": "internal" }
```

If the route had already begun its answer, the connection is cut instead, so a
half download never looks complete.
[Behavior worth knowing](/self-hosting/configuration/#behavior-worth-knowing)
says what the log records.

## Live events

Live updates left REST at 0.7.0. The graph serves them as subscriptions over
Server-Sent Events, so a client posts to `/api/graphql` with
`Accept: text/event-stream`. See
[subscriptions](/reference/graphql-api/#subscriptions).

For payloads and reliable delivery, subscribe a
[webhook](/reference/webhooks/) instead. The delivery contract did not move.
