---
title: Extending the graph
description: Add your plugin's own fields to AlphOne's GraphQL API, from an empty folder to a query answering.
---

AlphOne serves one GraphQL API at `POST /api/graphql`. Your plugin adds its own
fields to it, and callers read yours and the core's in a single query.

Two words you will meet. **GraphQL** is a query language where the caller lists
the fields it wants and gets exactly those back. A **resolver** is the Go
function that produces one field's value when somebody asks for it.

This page builds a whole plugin, `note`, that keeps a short note against a
contact. Copy it and rename it.

## The whole plugin

Four files. Nothing else is needed, and nothing has to be registered by hand.

`plugins/note/plugin.json` tells the build your plugin joins the graph:

```json
{
	"id": "note",
	"name": "Note",
	"backend": "github.com/gopherium/alphone/plugins/note",
	"graphql": true
}
```

`plugins/note/graph/schema.graphqls` is your slice of the API. `extend` adds to
a type the core already defines:

```graphql
extend type Query {
  notes(contactId: UUID): [Note!]!
}

type Note {
  id: UUID!
  contactId: UUID!
  body: String!
  createdAt: DateTime!
}

extend type Contact {
  notes: [Note!]! @goField(forceResolver: true)
}
```

`plugins/note/note.go` is the plugin itself:

```go
// Package note keeps a short note against a contact.
package note

// Plugin keeps a short note against a contact.
type Plugin struct {
	mu   sync.Mutex
	rows []noteRow
}

// Register builds the note [Plugin] from the host-provided deps.
func Register(sdk.Deps) (*Plugin, error) { return &Plugin{}, nil }

// ID returns the plugin's identifier.
func (p *Plugin) ID() string { return "note" }

// Start readies the plugin.
func (p *Plugin) Start(context.Context) error { return nil }

// Stop releases the plugin's resources.
func (p *Plugin) Stop(context.Context) error { return nil }
```

`plugins/note/graphql.go` holds the resolvers. You write one struct per
GraphQL type you touch, and a method on your plugin returning it:

```go
// QueryResolvers serves the plugin's Query root fields.
type QueryResolvers struct {
	plugin *Plugin
}

// QueryResolvers returns the plugin's Query resolver set.
func (p *Plugin) QueryResolvers() QueryResolvers {
	return QueryResolvers{plugin: p}
}

// Notes lists the stored notes, narrowed to one contact when asked.
func (q QueryResolvers) Notes(_ context.Context, contactID *uuid.UUID) ([]*model.Note, error) {
	return toGraphNotes(q.plugin.list(contactID)), nil
}

// ContactResolvers serves the plugin's fields on the core Contact type.
type ContactResolvers struct {
	plugin *Plugin
}

// ContactResolvers returns the plugin's Contact resolver set.
func (p *Plugin) ContactResolvers() ContactResolvers {
	return ContactResolvers{plugin: p}
}

// Notes lists the notes kept against one contact.
func (c ContactResolvers) Notes(_ context.Context, obj *model.Contact) ([]*model.Note, error) {
	return toGraphNotes(c.plugin.list(&obj.ID)), nil
}
```

`model.Note` does not exist yet. The next step writes it.

## Build it and ask it something

```sh
make generate
go build ./...
ALPHONE_DEV_GRAPHIQL=1 make dev
```

`make generate` reads your schema and writes the Go types, the wiring, and the
merged schema at `graph/schema.graphql`. Run it after every schema change.
Nothing under `graph/` is written by hand.

Open `http://localhost:8080/api/graphql`, log in, and ask:

```graphql
{
  notes {
    id
    body
    createdAt
  }
}
```

```json
{
  "data": {
    "notes": [
      {
        "id": "019f5a00-0000-7000-8000-0000000000a1",
        "body": "Called back, wants the renewal quote",
        "createdAt": "2026-08-09T16:21:31Z"
      }
    ]
  }
}
```

The field you added to `Contact` works the same way, and returns the notes whose
`contactId` matches that contact:

```graphql
query($id: UUID!) {
  contact(id: $id) {
    name
    notes {
      body
    }
  }
}
```

One query, one round trip, a core field and yours side by side.

## Three rules that bite

**Name every type after your plugin.** A type you define must start with your
plugin id, ignoring case. `note` may define `Note` and `NoteDraft`, not `Draft`.
`go test ./graph/` fails by name if you stray. A type that has to keep another
name goes in `plugins/note/testdata/unprefixedtypes.txt`, one name per line.
The same test fails when a line there names a type your schema no longer
defines.

**Fields you add to somebody else's type need `@goField(forceResolver: true)`.**
Without it the build looks for a `Notes` field on the core's `Contact` struct,
does not find one, and fails. With it, your resolver is asked instead. Fields on
your own types do not need it.

**Write one resolver struct per GraphQL type you touch.** The generator reads
your schema, works out which ones you owe, and writes an interface you have to
satisfy. Miss one and the build names it.

## Lists

A field that answers a list and takes a `limit` names it as the argument that
sizes the list:

```graphql
extend type Query {
  noteEntries(limit: Int, offset: Int): [NoteEntry!]!
    @scope(area: "notes", write: false)
    @listSize(slicingArguments: ["limit"])
}
```

Every query has a cost limit, see
[Limits](/reference/graphql-api/#limits). `@listSize` prices the field as the
rows it asks for times the cost of one row. A call with no `limit` is priced
at `ALPHONE_GRAPH_PAGE_SIZE` rows, the same page the core lists answer.
`go test ./graph/` fails when a plugin list takes a `limit` without
`@listSize`, or names an argument the field does not take.

Answer the same page the price assumes. `deps.Pages` carries the page size and
the largest page from `ALPHONE_GRAPH_PAGE_SIZE` and `ALPHONE_GRAPH_PAGE_CAP`.
Keep it from `Register` and resolve each call through it:

```go
// NoteEntries lists one page of the notes, newest first.
func (q QueryResolvers) NoteEntries(
	ctx context.Context, limit, offset *int,
) ([]*model.NoteEntry, error) {
	size, err := q.plugin.pages.Limit(limit)
	if err != nil {
		return nil, err
	}
	return q.plugin.store.listNotes(ctx, size, sdk.Offset(offset))
}
```

`Limit` answers the page size for a call with no `limit`. A `limit` below 1 or
past the largest page is refused with the code `VALIDATION` and the reason
`first_out_of_range`, the way the core lists refuse it. `sdk.Offset` answers 0
for a missing or negative `offset`.

## Going further

- **Returning a core type.** Build it with `id` and `name` only. The rest of
  `Contact` is resolver-backed, so the core fills `createdAt`, `identities` and
  `tasks` when a caller asks. You need the name yourself, and the SDK has no
  lookup by contact id, so read `core.contacts` from your own store the way
  `plugins/whatsapp` does.
- **Batching.** A field on a list resolves once per row. Keep one loader per
  request with `sdk.ScopedValue` and it fetches them in one query. See
  `internal/graphres/loaders.go`.
- **File uploads.** Use the `Upload` scalar. See
  `plugins/importer/graph/schema.graphqls`.
- **Staying inside the workspace.** One database can hold several
  workspaces, and every row carries its workspace in `tenant_id`. Each
  statement your plugin runs must filter by `tenant_id`, or write it when it
  inserts. `go test ./internal/postgres/` reads the Go code in your plugin
  folder and names a statement that does not. It reads only statements
  written as Go strings, so a plugin that keeps `migrations/` fails it when
  no Go string names one of its tables. A statement that must run
  before the workspace is known, such as a webhook finding which workspace a
  message is for, goes in `plugins/note/testdata/crosstenant.txt`. Each line
  holds a piece of its text that matches exactly one of your statements.
- **A fuller example.** `plugins/whatsapp/graph/` is the living reference.

## When it does not work

| Symptom | Cause |
| ------- | ----- |
| The build says your model has no field named after your added field | The field is missing `@goField(forceResolver: true)` |
| The build names a resolver set you have not written | Your schema asks for it, add the struct and the method returning it |
| `graphql plugins require a backend` | `plugin.json` says `graphql: true` with no `backend` |
| `go test ./graph/` names your type | It does not start with your plugin id |
| `go test ./graph/` says your list takes a limit without `@listSize` | Add `@listSize(slicingArguments: ["limit"])` to the field |
| Your field is missing from `graph/schema.graphql` | `make generate` was not run after the schema change |
