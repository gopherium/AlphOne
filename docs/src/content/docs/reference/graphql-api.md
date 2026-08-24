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

`login` and the `locale` query are the only operations an anonymous caller may
run. Anything else answers `UNAUTHENTICATED` with HTTP 200, because a GraphQL
error is not an HTTP error:

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
roles existed. `me` answers an empty `role` and an empty `capabilities` for it.
Such an account still signs in, still reads `me` and `logout`, and still works
every field that names no capability, which today is the whole product. What it
cannot reach is the fields a capability guards, which is user management. Give
it a role and it gains whatever that role holds.

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
        "capability": "manage_users",
        "reason": "capability_missing",
        "meta": { "scope": "users:write", "capability": "manage_users" }
      }
    }
  ],
  "data": null
}
```

The message reads `admin required` whichever capability was missing, because it
has said that since before capabilities existed and clients match on it. Read
`capability` rather than the message to learn what the account's role actually
fell short of. Holding the admin role is not what the field asks for, holding
that capability is, and a plugin-declared role holding it passes just as well.

The `scope` extension still names what the field wanted, so a caller always
learns which area an operation acts in. An operation refused over a token's
scopes carries no `capability`, so the two halves stay distinguishable. Minting a wider token
does not help here. A token cannot carry more authority than the user it acts
as.

Listing users stays open to members. A member sees who its colleagues are,
which is what assigning a task to one of them needs.

Nobody changes its own role, and nobody disables its own account. Both are
refused with code `VALIDATION` and the messages `you cannot change your own
role` and `you cannot disable your own account`.

Writing a role the caller does not hold itself is refused the same way, with
`that role is beyond your own`. A caller may only write a role whose
capabilities it already holds, and may only touch an account whose current role
it likewise holds. So an admin can neither grant a role reaching further than
admin nor demote or disable an account already holding one, whether that role
came with the product or with a plugin.

A write that would leave no enabled account able to manage users is refused with
`the last admin cannot be unseated`.

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

## Locale

AlphOne answers each reader in one locale. `locale` resolves it: the signed-in
account's stored choice wins, else the closest match to the `Accept-Language`
header, else `en-US`. The query is open to anonymous callers, so a login screen
can ask before anyone signs in.

```graphql
query { locale }
```

`supportedLocales` lists every locale AlphOne serves, the default first, so a
screen can offer the choice without hardcoding the list.

`setLocale` stores the calling account's choice and answers it back. It takes
a locale from the supported list and refuses anything else with the reason
`locale_unknown`, naming the list in `meta.supported`.

```graphql
mutation { setLocale(locale: "es-ES") }
```

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
      "extensions": { "code": "VALIDATION", "reason": "contact_name_required" }
    }
  ],
  "data": null
}
```

`path` names the field that failed, which matters when one operation asks for
several.

### Reasons

Beside the coarse `code`, a refused operation names a `reason`, a short fixed
name for the exact condition, and `meta`, the values its message mentions. A
client should match on `reason` and read `meta`, never parse the message. The
message can be reworded, a reason never is. An `INTERNAL` error names no
reason, its message and shape are deliberately bare.

```json
{
  "errors": [
    {
      "message": "graph: first must be between 1 and 200",
      "extensions": {
        "code": "VALIDATION",
        "reason": "first_out_of_range",
        "meta": { "min": 1, "max": 200 }
      }
    }
  ],
  "data": null
}
```

The reasons the core answers with:

| Reason | Meta | When |
| ------ | ---- | ---- |
| `authentication_required` | | no usable credential |
| `credentials_invalid` | | a login that did not match |
| `rate_limited` | `retryAfter` | too many attempts |
| `scope_missing` | `scope` | the token lacks the area |
| `capability_missing` | `scope`, `capability` | the role lacks the capability |
| `contact_name_required` | | a contact needs a name |
| `identity_channel_required` | | an identity needs a channel |
| `identity_identifier_required` | | an identity needs an identifier |
| `identity_taken` | `ownerContactId` | the identity belongs to another contact |
| `channel_not_writable` | | the channel accepts no writes |
| `identity_not_found` | | the id names no identity |
| `contact_not_found` | | the id names no contact |
| `task_title_required` | | a task needs a title |
| `task_priority_unknown` | | the priority is not one AlphOne knows |
| `task_status_unknown` | | the status is not one AlphOne knows |
| `task_filter_choice_required` | | tasks take exactly one filter |
| `task_not_found` | | the id names no task |
| `origin_source_required` | | an origin event needs a source |
| `event_unknown` | | the event name is not one AlphOne knows |
| `webhook_url_invalid` | | the webhook URL does not parse |
| `webhook_events_required` | | a webhook needs at least one event |
| `webhook_not_found` | | the id names no webhook |
| `first_out_of_range` | `min`, `max` | the page size is outside the range |
| `locale_unknown` | `supported` | the locale is not one AlphOne serves |
| `cursor_malformed` | | the cursor is not one a field issued |
| `value_malformed` | | a scalar did not parse |
| `token_name_required` | | a token needs a name |
| `token_not_found` | | the id names no token |
| `scope_malformed` | | a scope is area colon access |
| `scopes_required` | | a scoped token needs at least one |
| `area_unknown` | | the area is not one the schema declares |
| `lifetime_negative` | | a lifetime is zero or more days |
| `lifetime_too_long` | `maxDays` | the lifetime is past the cap |
| `email_invalid` | | the address does not parse |
| `email_taken` | | the address belongs to another account |
| `name_required` | | an account needs a name |
| `name_too_long` | `max` | the name is past the cap |
| `password_too_short` | `min` | the password is under the floor |
| `password_too_long` | `max` | the password is past the cap |
| `user_not_found` | | the id names no account |
| `self_disable_refused` | | nobody disables its own account |
| `self_role_refused` | | nobody changes its own role |
| `last_privileged_refused` | | the last account able to manage users stays |
| `role_beyond_reach` | | the role holds more than the caller does |
| `role_unknown` | | the role is not one the deployment names |

The stock plugins add their own:

| Reason | Meta | When |
| ------ | ---- | ---- |
| `field_name_malformed` | | a field name is camelCase |
| `field_label_required` | | a field needs a label |
| `field_kind_unknown` | | the kind is not one the plugin knows |
| `field_name_reserved` | | the name is already a column of the type |
| `field_name_taken` | | another definition holds the name |
| `field_kind_locked` | | an archived definition pins the kind |
| `field_not_found` | | the id names no live definition |
| `field_unknown` | | no live definition holds the name |
| `value_kind_mismatch` | | the value does not match the declared kind |
| `values_not_an_object` | | values arrive as an object of names |
| `message_content_required` | | a message needs text |
| `conversation_not_found` | | the id names no conversation |
| `upstream_failed` | | the messaging platform did not accept |
| `import_not_found` | | the id names no import |
| `file_too_large` | `maxBytes` | the upload is past the cap |
| `file_unreadable` | | the file is not a CSV or spreadsheet AlphOne reads |
| `mapping_invalid` | | the mapping does not fit the columns |
| `mapping_required` | | committing needs a mapping first |
| `mapping_locked` | | the import no longer accepts a mapping |
| `already_committed` | | the import was committed before |

A plugin you install may add more, each documented by the plugin.

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
| Locale | `locale`, `supportedLocales` | `setLocale` |
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
