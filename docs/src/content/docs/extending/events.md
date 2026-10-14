---
title: Events
description: Declare the events your plugin publishes, so webhooks can subscribe to them.
---

Your plugin can announce that something happened, such as a message
that arrived or an import that finished. AlphOne passes each event to
the webhooks subscribed to it, see [Webhooks](/reference/webhooks/).

A plugin declares every event name it publishes before it publishes
one. AlphOne then knows the full list of names at start, so a webhook
can subscribe to a plugin event, and a typo in a name is caught.

This page uses one plugin, `archive`, that announces each purge.

## Declare your events

A plugin declares its events by implementing `sdk.EventProvider`,
whose one method returns the names:

```go
// eventPurged names the event a finished purge announces.
const eventPurged = "archive.rows.purged"

var _ sdk.EventProvider = (*Plugin)(nil)

// Events returns the event names the archive plugin publishes.
func (p *Plugin) Events() []string {
	return []string{eventPurged}
}
```

The `var _` line turns a missing or misspelled `Events` method into a
build error. Without it AlphOne finds no declaration and drops every
event the plugin publishes.

A name is two or more parts joined by dots. Each part starts with a
lower-case letter and holds only lower-case letters, digits and
underscores. Start the name with your plugin's id, so it cannot clash
with another plugin's. `archive.rows.purged` follows both rules.

## Publish one

`deps.Events` publishes an event. Keep it from `Register`, then call
`Publish` from `Start` onwards, or while serving a request, with the
name and the data a subscriber reads:

```go
p.events.Publish(ctx, eventPurged, map[string]any{
	"deleted": deleted.RowsAffected(),
})
```

Each delivery carries the data under `data`, see
[the envelope](/reference/webhooks/#the-envelope). Put enough in it to
identify what happened, such as an id, and let the subscriber read the
rest through the API.

Use the same constant in `Events` and in `Publish`, so the two cannot
drift apart. A name no plugin declared is dropped, and the log gets an
error, `a plugin published an event it never declared`, naming the
event. A core event such as `task.created` is dropped the same way,
because only AlphOne itself publishes those. So is an event published
inside `Register`, since AlphOne reads the declarations only after every
plugin has registered.

## When a declaration is refused

AlphOne reads every plugin's `Events` when it loads the plugins, and
stops with an error when a name cannot be declared:

| Declared name | Error |
| --- | --- |
| Breaks the name rules, such as `Archive.Purged` or `purged` | `event: malformed name: "purged" declared by archive` |
| A core event, such as `task.created` | `event: name taken: "task.created" declared by archive is a core event` |
| A name another plugin declared | `event: name taken: "archive.rows.purged" declared by archive is held by cleanup` |

The error starts with `declare plugin events:`. Every command that
loads the plugins stops with it, `serve` included, so the clash is
found before anything runs. The same plugin may list one name twice.

## The names a webhook can take

The `webhookEvents` query lists every name a webhook can subscribe to,
the core events first and then each declared plugin event in
alphabetical order:

```graphql
query { webhookEvents }
```

A stock install answers `task.created`, `task.completed`,
`contact.created`, `import.completed` and `whatsapp.message.received`.
Once the archive plugin is built in, `archive.rows.purged` joins them,
and `createWebhook` accepts it.
