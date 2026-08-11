---
title: GraphQL API
description: The endpoint, how to authenticate, and how reads, writes, paging and errors work.
---

AlphOne serves one GraphQL API. Everything the interface does goes through it,
so anything the app can do, your program can do.

```text
POST https://your-domain/api/graphql
Content-Type: application/json
```

Send `{"query": "...", "variables": {...}}` and read `data` and `errors` back.

An AI agent can read AlphOne without writing queries, see
[AI agents](/guides/agents/).

## Authenticating

Two credentials work, and every operation needs one.

**An API token**, for programs. Create one on the AlphOne host and send it as a
bearer token:

```sh
alphone token create -email you@example.com -name "my integration"
```

```text
Authorization: Bearer a1_...
```

A token acts as the user who created it. Work it creates records
`token:<name>` in `originSource`, so automated work stays distinguishable from
typed work.

**A session cookie**, for browsers. Call `login` and the reply sets it:

```graphql
mutation {
  login(email: "admin@example.com", password: "password1234") {
    me { id email name }
  }
}
```

```json
{
  "data": {
    "login": {
      "me": {
        "id": "019fdd4b-b6a3-710d-85dd-b597d7a9b453",
        "email": "admin@example.com",
        "name": "Admin"
      }
    }
  }
}
```

`login` is the only operation an anonymous caller may run. Anything else
answers `UNAUTHENTICATED` with HTTP 200, because a GraphQL error is not an HTTP
error:

```json
{
  "errors": [
    { "message": "authentication required", "extensions": { "code": "UNAUTHENTICATED" } }
  ],
  "data": null
}
```

Always read `errors`. A 200 does not mean it worked.

## Scalars

| Scalar | Format | Example |
| ------ | ------ | ------- |
| `UUID` | RFC 4122 text | `0198d000-0000-7000-8000-000000000001` |
| `DateTime` | RFC 3339 in UTC | `2026-08-09T16:21:31Z` |
| `Date` | calendar day, no time | `2026-08-06` |
| `Upload` | multipart file part | see uploads below |

## Paging

Lists that can grow are connections. You ask for `first` and walk with `after`.

```graphql
query($before: Date!, $first: Int!, $after: String) {
  tasks(dueBefore: $before, status: "all", first: $first, after: $after) {
    edges {
      cursor
      node { id title dueOn status }
    }
    pageInfo { hasNextPage endCursor }
  }
}
```

```json
{
  "data": {
    "tasks": {
      "edges": [
        {
          "cursor": "eyJkdWVfb24iOiIyMDI2LTA4LTA2IiwiaWQiOiIwMTk4ZDAwMC0wMDAwLTcwMDAtODAwMC0wMDAwMDAwMDAwMDEifQ",
          "node": {
            "id": "0198d000-0000-7000-8000-000000000001",
            "title": "Chase the overdue invoice",
            "dueOn": "2026-08-06",
            "status": "open"
          }
        }
      ],
      "pageInfo": {
        "hasNextPage": true,
        "endCursor": "eyJkdWVfb24iOiIyMDI2LTA4LTA2IiwiaWQiOiIwMTk4ZDAwMC0wMDAwLTcwMDAtODAwMC0wMDAwMDAwMDAwMDEifQ"
      }
    }
  }
}
```

Pass `endCursor` back as `after` for the next page, and stop when
`hasNextPage` is false. Treat a cursor as opaque, it is only meaningful to the
field that issued it. `first` accepts 1 to 200 and defaults to 50.

Listing tasks takes exactly one of `date`, `dueBefore` or `contactId`. Sending
none or two answers `VALIDATION`.

## Writing

```graphql
mutation($input: CreateTaskInput!) {
  createTask(input: $input) {
    replay
    task { id title dueOn status }
  }
}
```

```json
{
  "data": {
    "createTask": {
      "replay": false,
      "task": {
        "id": "019fe75e-4cc1-715f-8a50-dfeff9695b98",
        "title": "Read the reference",
        "dueOn": "2026-08-10",
        "status": "open"
      }
    }
  }
}
```

Give `CreateTaskInput` an `originEventId` and the creation becomes repeatable.
A second create with the same origin returns the task already stored and
`replay: true`, rather than a duplicate.

## Errors

Every error carries a `code` in its `extensions`.

| Code | Meaning |
| ---- | ------- |
| `UNAUTHENTICATED` | No usable credential, or the operation is not `login` |
| `VALIDATION` | The input was refused. `message` names the field or rule |
| `NOT_FOUND` | The id names nothing |
| `CONFLICT` | An identity is already claimed. `ownerContactId` names the owner |
| `RATE_LIMITED` | Too many attempts. `retryAfter` is in seconds |
| `COMPLEXITY_LIMIT_EXCEEDED` | The query asks for too much, see limits below |
| `INTERNAL` | AlphOne failed. The message is deliberately bare |

A refused input looks like this:

```json
{
  "errors": [
    {
      "message": "contact: empty name",
      "path": ["createContact"],
      "extensions": { "code": "VALIDATION" }
    }
  ],
  "data": null
}
```

`path` names the field that failed, which matters when one operation asks for
several.

## Limits

| Limit | Value |
| ----- | ----- |
| Query complexity | 2500 per operation |
| Concurrent operations | 20 per user |
| Concurrent subscriptions | 5 per user |
| Operation deadline | 60 seconds |
| JSON body | 1 MiB |
| Multipart body | 6 MiB |

Complexity prices a page as the number of rows asked for times the cost of one
row, so a wide selection over a large page is what trips it:

```json
{
  "errors": [
    {
      "message": "operation has complexity 2800, which exceeds the limit of 2500",
      "extensions": { "code": "COMPLEXITY_LIMIT_EXCEEDED" }
    }
  ],
  "data": null
}
```

Ask for fewer rows or fewer fields. Over a budget, AlphOne answers HTTP 429
with a `Retry-After` header.

## Subscriptions

Subscriptions arrive over Server-Sent Events on the same endpoint. Send
`Accept: text/event-stream` and no JSON accept type, or you get a JSON answer
instead of a stream.

```graphql
subscription {
  coreEvent
}
```

`coreEvent` streams the names of events you may see. Task events reach only
their assignee. A stream is closed after 5 minutes, so reconnect and refresh.

## Uploads

A mutation taking a file uses `multipart/form-data` following the GraphQL
multipart request specification, rather than plain JSON. `importUpload` is the
one that ships.

## Introspection

Introspection is on, so any GraphQL client can read the schema and give you
completion. The interactive query page is off by default and enabled with
`ALPHONE_DEV_GRAPHIQL`, for development only.

```graphql
{
  __schema { queryType { name } }
}
```

The full field list lives in the schema itself rather than on this page, so it
cannot drift. Point a client at the endpoint, or read
`graph/schema.graphql` in the repository.

## What you can ask for

| Area | Reads | Writes |
| ---- | ----- | ------ |
| Session | `me` | `login`, `logout` |
| Users | `users` | `createUser`, `setUserDisabled` |
| Contacts | `contacts`, `contact` | `createContact`, `renameContact`, `addContactIdentity`, `deleteContactIdentity` |
| Tasks | `tasks`, `task` | `createTask`, `updateTask` |
| Webhooks | `webhooks` | `createWebhook`, `deleteWebhook` |
| Imports | `imports`, `importJob`, `importFields` | `importUpload`, `importSetMapping`, `importCommit` |
| WhatsApp | `whatsAppConversations`, `whatsAppConversation` | `whatsAppSendMessage` |
| Version | `version` | |

Subscriptions are `coreEvent`, `whatsAppConversationEvent` and
`whatsAppMessageReceived`.

Plugins add their own fields, so an instance may serve more than this. See
[extending the graph](/extending/graph/).
