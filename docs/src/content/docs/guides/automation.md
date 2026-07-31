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

:::note[AlphOne does not include an engine]
AlphOne neither bundles nor redistributes n8n, and the released image
contains only AlphOne itself. You run the engine you choose, under its
own licence, and point it at the API. Swapping engines later changes
nothing on the AlphOne side.
:::

## Mint a token

An engine cannot log in, so give it an API token. See the
[REST API reference](/reference/rest-api/) for the full auth rules.

```sh
alphone token create -email you@example.com -name "n8n production"
```

The secret prints once. Copy it now, because it is stored only as a hash
and cannot be recovered, only replaced.

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

## Verify the credential

Point an HTTP Request node at `GET /api/version`. It needs a valid
credential and returns a small body, which makes it a good connection
test:

```json
{ "version": "0.4.2" }
```

## A first automation

A morning digest of overdue work needs three nodes: a Schedule Trigger, an
HTTP Request, and a Code node.

The HTTP Request node calls `GET /api/tasks` with these query parameters:

| Parameter | Value |
| --------- | ----- |
| `due_before` | `{{ $now.toFormat('yyyy-MM-dd') }}` |
| `status` | `open` |
| `limit` | `200` |

The Code node turns the response into a message:

```js
const tasks = $input.first().json.tasks ?? [];
const lines = tasks.map((t) => `- ${t.title} (due ${t.due_on})`);
return [{ json: { overdue: tasks.length, digest: lines.join('\n') } }];
```

Swap the Code node for Slack, Gmail, or Telegram to deliver it.

## Writing back

A token has the same permissions as the user who created it, so an engine
can create and update records too. `POST /api/tasks` with a JSON body
creates work:

```json
{ "title": "Follow up on the renewal", "due_on": "2026-08-03" }
```

Tasks created with an API token record the token automatically in
`origin_source` as `token:<name>`, which keeps automated work
distinguishable from work a person typed. Attribution comes from the
credential rather than the request body, so it cannot be faked and it
needs nothing from the workflow.
