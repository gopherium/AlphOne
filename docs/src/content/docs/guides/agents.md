---
title: AI agents
description: Connect an AI agent to AlphOne over MCP, so it can read your tasks and contacts.
---

This page connects an AI (Artificial Intelligence) agent to AlphOne, so you
can ask it things like how many tasks you have today, or which contacts are
waiting on you.

AlphOne speaks MCP (Model Context Protocol), an open standard for connecting
AI applications to tools and data. Any MCP client can use it. The examples
below use Claude Code because it is the one we run.

## What you need

- AlphOne 0.9.0 or newer, reachable from where the agent runs
- An AlphOne login, to mint a token
- An MCP client that can send an `Authorization` header

## 1. Mint a token

The agent acts as the user who owns the token, so mint it against the account
whose work you want the agent to read.

```sh
alphone token create -email you@example.com -name "my agent"
```

The secret starts with `a1_` and is shown once. Store it now.

## 2. Connect the agent

The endpoint is `POST /api/mcp` on your AlphOne. In Claude Code:

```sh
claude mcp add --transport http alphone https://your-domain/api/mcp \
  --header "Authorization: Bearer a1_your_token_here"
```

Check it connected with `/mcp`, which lists the tools AlphOne offers.

For the Claude Agent SDK (Software Development Kit), the same endpoint and
header go in the `mcpServers` option:

```ts
mcpServers: {
  alphone: {
    type: "http",
    url: "https://your-domain/api/mcp",
    headers: { Authorization: `Bearer ${process.env.ALPHONE_TOKEN}` },
  },
}
```

:::caution[Bearer tokens only]
The claude.ai and Claude Desktop connector screens expect OAuth (Open
Authorization). Their request header option, which could carry AlphOne's
token, is still in beta and not available to everyone. So the endpoint works
from Claude Code and the Agent SDK today, and from those two only once that
feature reaches you.
:::

## 3. Ask it something

```
How many tasks do I have today?
Which of my contacts have open work?
What is on my plate for the first of September?
```

The agent picks a tool, calls it, and words the answer itself.

## The tools

Four tools, and none of them writes. Each is marked read only, so a client
can tell without trying.

| Tool | Answers |
| ---- | ------- |
| `workload_summary` | How many open tasks are due today, and how many are overdue |
| `list_my_tasks` | The tasks for one day, or the overdue backlog, with contact names |
| `find_contacts` | Contacts matching a search, each marked whether it holds open work |
| `get_contact` | One contact in full, with its channel addresses and open tasks |

Every tool answers structured data, never a written sentence. The wording of
an answer is the agent's, which is why the same tool reads naturally in any
language the agent speaks.

Two details worth knowing when you read an answer:

**Counts stop at 200.** When there are more, the answer sets `capped` to
true and the number means at least that many.

**Task lists are yours, contact answers are shared.** `workload_summary` and
`list_my_tasks` read only the token owner's tasks. The two contact tools show
a contact's open work across every user, exactly like the contact page in the
web app, and each task says whose it is in `assignee_id`.

## Reaching further

The tools cover the common questions. Anything else AlphOne can do is
reachable over the [GraphQL API](/reference/graphql-api/), which an agent can
call through a plain HTTP request tool.

The list is short on purpose. Agents choose well among a few clearly
described tools and badly among many, so AlphOne exposes the jobs an agent
actually does, not one tool per operation.

## Security

The MCP endpoint takes the same API tokens as the rest of AlphOne, so
everything under [authenticating](/reference/graphql-api/#authenticating)
applies. A token acts as its user, disabling that user stops the token, and
the secret is stored hashed and cannot be recovered.

Give an agent its own token rather than sharing one with an automation
engine. Revoking it then costs nothing else.
