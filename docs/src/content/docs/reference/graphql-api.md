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

A token acts as the user who created it, and never reaches further than that
user does. See [Roles](#roles) for what a role may do. Work it creates
records `token:<name>` in `originSource`, so automated work stays
distinguishable from typed work.

A token narrows that authority twice over. Each `-scope` grants one area
and whether the token may write there, and an operation touching an area
the token does not hold is refused with code `UNAUTHORIZED` naming the
scope it needed. Without `-scope` the token holds every area. A newly
minted token also expires, after ninety days unless `-ttl` says otherwise,
and an expired token is refused as `invalid token`. Tokens that predate
scopes keep full authority and no expiry until you replace them, so run
`alphone token list` to see which ones those are.

Token management is the one thing a token can never do, whatever its
scopes. `apiTokens`, `apiTokenCreate` and `apiTokenRevoke` require a
session, so a leaked token cannot mint its own replacement.

A scope grants an entry point and everything that entry point reaches.
The check runs on the fields an operation starts from, not on every
field it walks through, so `contacts:read` reads a contact's open tasks
through `contact { tasks { ... } }` without holding `tasks:read`. Grant
the narrowest set of entry points an integration needs, and read a
scope as the doorway it opens rather than a fence around one table.

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

## Roles

An account holds one role, and the role decides what the account may do. A stock
deployment names two. An admin manages users. A member works the product, which
is contacts, tasks, and whatever your plugins add. A plugin may declare roles of
its own, so do not assume the list stops at two.

An account can also hold no role at all, which happens to accounts made before
roles existed. Such an account holds no capability, so it can do nothing until
somebody gives it a role. `me` answers an empty `role` and an empty
`capabilities` for it.

What a role may do is a set of named capabilities. `me` answers the ones the
calling account holds, so a client asks what it may do rather than guessing from
the role's name:

```graphql
query { me { role capabilities grantable } }
```

`capabilities` names what the account may do. `grantable` names the roles it may
give another account, which is every role whose capabilities it already holds
itself. An admin cannot grant a role that reaches further than its own.

Three operations need the `manage_users` capability: `createUser`,
`setUserDisabled` and `setUserRole`. An account without it is refused:

```json
{
  "errors": [
    {
      "message": "admin required",
      "extensions": {
        "code": "UNAUTHORIZED",
        "scope": "users:write",
        "capability": "manage_users"
      }
    }
  ],
  "data": null
}
```

The `scope` extension still names what the field wanted, so a caller always
learns which area an operation acts in, and `capability` names what the
account's role fell short of. A refusal about a token's scopes carries no
`capability`, so the two halves stay distinguishable. Minting a wider token
does not help here. A token cannot carry more authority than the user it acts
as.

Listing users stays open to members. A member sees who its colleagues are,
which is what assigning a task to one of them needs.

Nobody changes its own role, and nobody disables its own account. Both are
refused with code `VALIDATION` and the messages `you cannot change your own
role` and `you cannot disable your own account`. Together they keep a
deployment from losing its last admin, since an admin can only ever demote
somebody else, and there is always itself left holding the authority.

Writing a role the caller does not hold itself is refused the same way, with
`that role is beyond your own`. So an admin can neither grant a role reaching
further than admin nor touch an account already holding one.

`createUser` takes an optional `role`. Leaving it out starts the account at the
narrowest role the deployment names, which is `member` in a stock install.

### Roles and scopes together

A role narrows the user. A scope narrows what a token carries of that user's
authority. An operation runs only when both allow it.

| The caller | What it holds | Reaching `createUser` |
| ---------- | ------------- | --------------------- |
| An admin's session | the capability, and no token to narrow it | yes |
| An admin's token scoped `users:write` | both | yes |
| An admin's token scoped `contacts:read` | the capability but not the scope | no, `scope required: users:write` |
| A member's token scoped `*` | the scope but not the capability | no, `admin required` |

The token is checked first. A caller holding neither is told about the scope,
because that is the half it can fix on its own.

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
| `UNAUTHORIZED` | The caller does not reach the field. `scope required` means the token lacks the scope `scope` names, `admin required` means the account's role holds no capability the field needs |
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
| Users | `users` | `createUser`, `setUserDisabled`, `setUserRole` |
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
