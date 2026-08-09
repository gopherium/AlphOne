---
title: REST API
description: What still answers on REST, and the graph operation that replaced each route that is going away.
---

The [GraphQL API](/reference/graphql-api/) is AlphOne's API. Write new
integrations against it.

The REST routes below still answer, so nothing you already built stops
working today. They are being retired, and this page exists to tell you what
replaced each one. Two routes are staying, and they are listed at the end.

## Still answering, already replaced

Every route here works now and will be removed. Move to the operation beside
it. Reads and writes are all `POST /api/graphql`, so the whole table collapses
to one address once you have moved.

| REST route | Graph operation |
| ---------- | --------------- |
| `POST /api/auth/login` | `login` |
| `POST /api/auth/logout` | `logout` |
| `GET /api/auth/session` | `me` |
| `GET /api/contacts` | `contacts` |
| `POST /api/contacts` | `createContact` |
| `GET /api/contacts/{id}` | `contact` |
| `PATCH /api/contacts/{id}` | `renameContact` |
| `POST /api/contacts/{id}/identities` | `addContactIdentity` |
| `DELETE /api/contacts/{id}/identities/{identityId}` | `deleteContactIdentity` |
| `GET /api/tasks` | `tasks` |
| `POST /api/tasks` | `createTask` |
| `GET /api/tasks/{id}` | `task` |
| `PATCH /api/tasks/{id}` | `updateTask` |
| `GET /api/users` | `users` |
| `POST /api/users` | `createUser` |
| `PATCH /api/users/{id}` | `setUserDisabled` |
| `GET /api/webhooks` | `webhooks` |
| `POST /api/webhooks` | `createWebhook` |
| `DELETE /api/webhooks/{id}` | `deleteWebhook` |
| `GET /api/version` | `version` |
| `GET /api/plugins/whatsapp/conversations` | `whatsAppConversations` |
| `GET /api/plugins/whatsapp/conversations/{id}/messages` | `whatsAppConversation.messages` |
| `POST /api/plugins/whatsapp/conversations/{id}/messages` | `whatsAppSendMessage` |
| `GET /api/plugins/importer/fields` | `importFields` |
| `POST /api/plugins/importer/imports` | `importUpload` |
| `GET /api/plugins/importer/imports` | `imports` |
| `GET /api/plugins/importer/imports/{id}` | `importJob` |
| `PUT /api/plugins/importer/imports/{id}/mapping` | `importSetMapping` |
| `GET /api/plugins/importer/imports/{id}/rows` | `importJob.rows` |
| `GET /api/plugins/importer/imports/{id}/contacts` | `importJob.contacts` |
| `POST /api/plugins/importer/imports/{id}/commit` | `importCommit` |

Field names differ. REST answers `due_on` and `origin_event_id`, the graph
answers `dueOn` and `originEventId`. Lists that were a plain array with a
`next_cursor` are connections on the graph, with rows under `edges` and each
row under `node`. See [paging](/reference/graphql-api/#paging).

While these routes serve, they take the same two credentials the graph takes,
a session cookie or an `Authorization: Bearer` token. See
[authenticating](/reference/graphql-api/#authenticating).

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
