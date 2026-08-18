---
title: Automation
description: Drive AlphOne from an automation engine such as n8n.
---

AlphOne has no built-in rules engine. Anything the UI can do, the API can
do, so an automation engine drives AlphOne the same way the frontend
does: over HTTP, with a token.

Nothing in AlphOne is specific to any one engine. The examples below use
[n8n](https://n8n.io) because it is the one we run, but Activepieces,
Windmill, Node-RED, or a cron job with curl all work the same way.

There is a community node for n8n, which replaces the HTTP plumbing on
this page with two ready made nodes. To use it, follow the [n8n
guide](/guides/n8n/) instead. The rest of this page shows the plain HTTP
approach that any engine can take.

To connect an AI agent instead of an automation engine, see
[AI agents](/guides/agents/).

:::note[AlphOne does not include an engine]
AlphOne neither bundles nor redistributes n8n, and the released image
contains only AlphOne itself. You run the engine you choose, under its
own licence, and point it at the API. Swapping engines later changes
nothing on the AlphOne side.
:::

## Mint a token

An engine cannot log in, so give it an API token. See the
[GraphQL API reference](/reference/graphql-api/) for the full auth rules.

```sh
alphone token create -email you@example.com -name "n8n production" \
  -scope meta:read -scope webhooks:write -scope tasks:write -scope contacts:read
```

An engine needs `meta:read` for its credential test and `webhooks:write` for any
trigger that registers a subscription, beside the areas its own steps touch.

The secret prints once. Copy it now, because it is stored only as a hash
and cannot be recovered, only replaced.

Each `-scope` names an area the token may act in, and whether it may
write there. Area names are exact, so a typo is refused when you mint
the token. Without `-scope` the token gets every area, though it still
never reaches further than the person who created it, so a token minted
by a member does not gain user management. Without `-ttl`
it lasts ninety days, then answers `invalid token`. Pass `-ttl never`
for a token that does not expire.

## Run n8n locally

The source repository carries a scratch n8n behind its own compose
profile, for working on the integration. It is a development
convenience, not part of AlphOne, and the default stack leaves it
stopped:

```sh
make n8n        # http://localhost:5678
make n8n-down
```

The first visit asks you to create an owner account. That account is
local to the container and involves no n8n cloud service.

## Point n8n at AlphOne

In n8n, create a **Header Auth** credential:

| Field | Value |
| ----- | ----- |
| Name | `Authorization` |
| Value | `Bearer a1_your_token_here` |

Then use it from an **HTTP Request** node with Authentication set to
Generic Credential Type, Header Auth.

The base URL depends on where AlphOne runs relative to the container:

| AlphOne runs | Base URL from inside n8n |
| ------------ | ------------------------ |
| On your machine, n8n in Docker Desktop | `http://host.docker.internal:8080` |
| On your machine, n8n in Docker Engine on Linux | `http://host.docker.internal:8080`, with `extra_hosts: ["host.docker.internal:host-gateway"]` on the n8n service |
| Both in the same compose project | `http://alphone:8080` |
| A deployed instance | its public URL |

`localhost` inside the container means the container itself, never your
machine, so it will not reach AlphOne.

On Docker Engine for Linux the host must also be reachable, which can
mean binding the API beyond loopback with `ALPHONE_ADDR=0.0.0.0:8080`.
Do that only on a development machine, and mind that it exposes the API
to your network. Docker Desktop forwards loopback for you, so the
default `localhost:8080` is already reachable there.

## Every call looks the same

AlphOne serves one GraphQL API, so every request is a `POST` to the same
address with a JSON body naming what you want. There are no per resource
URLs to look up.

```text
POST <base url>/api/graphql
Content-Type: application/json
```

```json
{ "query": "...", "variables": {} }
```

Read the answer from `data`, and always check `errors`, because a refused
operation still answers `200`.

## Verify the credential

Point an HTTP Request node at the endpoint and ask for the version. It
needs a valid credential, which makes it a good connection test:

```json
{ "query": "{ version }" }
```

```json
{ "data": { "version": "%VERSION%" } }
```

## A first automation

A morning digest of overdue work needs three nodes: a Schedule Trigger, an
HTTP Request, and a Code node.

The HTTP Request node sends:

```json
{
  "query": "query($before: Date!) { tasks(dueBefore: $before, status: \"open\", first: 200) { edges { node { title dueOn } } } }",
  "variables": { "before": "{{ $now.toFormat('yyyy-MM-dd') }}" }
}
```

A list comes back as a connection, so the rows sit under `edges` and each
row under `node`:

```js
const edges = $input.first().json.data.tasks.edges ?? [];
const lines = edges.map((e) => `- ${e.node.title} (due ${e.node.dueOn})`);
return [{ json: { overdue: edges.length, digest: lines.join('\n') } }];
```

Swap the Code node for Slack, Gmail, or Telegram to deliver it.

## Writing back

A token has the same permissions as the user who created it, so an engine
can create and update records too:

```json
{
  "query": "mutation($input: CreateTaskInput!) { createTask(input: $input) { task { id title } } }",
  "variables": { "input": { "title": "Follow up on the renewal", "dueOn": "2026-08-12" } }
}
```

Tasks created with an API token record the token automatically in
`originSource` as `token:<name>`, which keeps automated work
distinguishable from work a person typed. `originSource` comes from the
credential rather than the request body, so it cannot be faked and it
needs nothing from the workflow.

## Creating the same task twice

Delivery is at least once, so a workflow can run twice for one event. A
plain create would then leave two identical tasks. Send the event you are
reacting to as `originEventId`, and the second create returns the task the
first one made with `replay: true`:

```json
{
  "input": {
    "title": "Follow up on the renewal",
    "dueOn": "2026-08-12",
    "originEventId": "0198d000-0000-7000-8000-0000000000e1"
  }
}
```

```json
{
  "data": {
    "createTask": {
      "replay": true,
      "task": { "id": "019fe769-7df3-7cf8-be91-7242a58e357a" }
    }
  }
}
```

Any uuid your engine can reproduce for the same piece of work will do.
The envelope `id` of the event you received is the usual choice, because
it names the event and is the same in every delivery and every retry. The
key is scoped by the token's name and by the user the token acts as, so
give your tokens distinct names.

## Turning an import into a call list

Importing a spreadsheet of contacts is only half the job. The work is
calling them, and a thousand new contacts is not a thousand tasks for
today. This recipe spreads them over as many days as it takes.

A spreadsheet column can also fill a field you defined yourself, so the
contacts arrive with a birth date or a loyalty score already on them. See
[Fields](/guides/fields/).

Subscribe to `import.completed`. It carries the import `id`, so the next
step reads what that import produced:

```graphql
query($id: UUID!) {
  importJob(id: $id) {
    contacts { contactId name rowId }
  }
}
```

Every entry carries `contactId`, `name`, and `rowId`, the row of the file
that created the contact:

```json
{
  "data": {
    "importJob": {
      "contacts": [
        {
          "contactId": "019fdd4b-b6ce-7dc1-af71-313ea3825797",
          "name": "Maria Perez",
          "rowId": "0198d000-0000-7000-8000-0000000000b1"
        }
      ]
    }
  }
}
```

Create one task per entry, with `contactId` linking the task to the person
and `rowId` as the `originEventId`:

```json
{
  "input": {
    "title": "Call Maria Perez",
    "dueOn": "2026-08-12",
    "contactId": "019fdd4b-b6ce-7dc1-af71-313ea3825797",
    "originEventId": "0198d000-0000-7000-8000-0000000000b1"
  }
}
```

The `rowId` is what makes this safe to re-run. A workflow that fails
halfway through, or a delivery that arrives twice, creates no second task
for a row that already has one, because AlphOne answers the repeat with
the task it already stored.

To spread the calls, batch the entries and push each batch a day further
out. Twenty per batch with `dueOn` set to today plus the batch index
gives twenty calls a day until the list runs out. The batch size is the
only number to change if the daily load is wrong.

Only rows that produced a contact appear here, so skipped duplicates and
failed rows create no work. The counts in the event tell you how many
each import produced.

The community node ships this recipe as a ready made workflow,
`import-to-daily-tasks.json` in the package's `examples` folder, which
you can import from the n8n canvas and point at your own credential. The
[n8n guide](/guides/n8n/) covers installing the node.
