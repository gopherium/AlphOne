---
title: Building a screen
description: The template every AlphOne screen follows, and the tests that enforce it.
---

Every screen in AlphOne is built from one template, so a screen your
plugin adds looks and behaves like a screen that ships with the product.
Import everything from `@alphone/frontend-sdk`, which serves the shared
admin kit alongside AlphOne's own pieces. Most of the template is
enforced by tests rather than by review.

Classes named `godmin-` come from that kit, so they behave the same in
every application built on it. Classes named `alphone-` are this
product's own.

## Your plugin's frontend

Your screens live in an npm package inside your plugin folder, at
`plugins/<id>/frontend`. Its `index.ts` exports a `FrontendPlugin`
named `plugin`. Put the package's name under `frontend` in your
`plugin.json`:

```json
{
  "id": "invoices",
  "name": "Invoices",
  "backend": "github.com/gopherium/alphone/plugins/invoices",
  "frontend": "@alphone/plugin-invoices"
}
```

Then run two commands from the repository root:

```sh
make generate
pnpm install
```

`make generate` adds your package to `frontend/package.json` and to
`frontend/src/plugins/index.ts`, the list of plugins the app loads. It
also writes the typed GraphQL client your queries import, in
`plugins/invoices/frontend/gql/`. `pnpm install` links your package into
the app and records it in `pnpm-lock.yaml`.

Commit all of these with your plugin. CI fails when any of them is out
of date. You never edit `frontend/package.json` or `codegen.ts` for
your plugin.

## Your plugin's styles

Most screens need no CSS of their own, because the shared kit already
draws them. When yours does, put the rules in one stylesheet at the root
of your plugin's `frontend` folder, such as
`plugins/invoices/frontend/invoices.css`. Never edit
`frontend/src/index.css` for your plugin.

- Import the sheet from each lazy screen file that draws its classes,
  never from `index.ts`. The admin loads every plugin's `index.ts`
  before first paint, so a sheet imported there would grow the first
  paint. A sheet imported by a screen loads with that screen's code.
- Start every class you write with `alphone-` and your plugin's id,
  such as `alphone-invoices-total`. A sheet stays loaded for the rest of
  the visit once a screen brought it in, so a class name two plugins
  share would take both plugins' rules. When a rule has to reach a class
  of the shared kit, start its selector with a class of your own, such
  as `.alphone-invoices-list .dataviews-view-table__cell-content-wrapper`,
  so it reaches your screen alone.
- Put the rules for a phone in one `@media` block at the end of the
  sheet, at the width where your screen changes its layout. That is
  `(max-width: 639px)`, the width where the admin turns to its phone
  layout, unless your screen changes at a width of its own. A list that
  keeps one column up to 782px uses `(max-width: 782px)`. Coming last,
  the rules in the block win over the rules above them without a longer
  selector.

The `lint:css` script of `frontend/package.json` reads every sheet at
the root of a plugin's `frontend` folder, with the same rules as the
app's own sheet. A sheet in a folder below it is not read. To test a
rule, read the sheet's text with `readFileSync` and pass it to
`declarationsIn` from `@alphone/frontend-sdk/testing`. It answers the
declarations of one selector, at the top level of the sheet or inside
the media block you name:

```ts
import { readFileSync } from 'node:fs'
import { join } from 'node:path'

import { declarationsIn } from '@alphone/frontend-sdk/testing'
import { expect, test } from 'vitest'

const sheet = readFileSync(join(import.meta.dirname, '..', 'invoices.css'), 'utf8')

test('hides the total on a phone', () => {
	expect(declarationsIn(sheet, '.alphone-invoices-total', '(max-width: 639px)'))
		.toEqual({ display: 'none' })
})
```

`declarationsIn` reads the sheet as the app's own sheet is written. Each
rule starts its own line as `selector {`, and the rules inside a media
block are indented with one tab. It does not find a rule indented with
spaces, and answers no declarations for it.

## The contract

A screen puts its title top left and its actions top right. The
sidebar title is the admin's only first level heading, so the page
title is the page's only second level heading. `PageScreen` with
`headingLevel={2}` gives you that shape, and every heading inside the
page moves one level down with it.

```tsx
import { Button, PageScreen } from '@alphone/frontend-sdk'
import { Link } from '@tanstack/react-router'

export function InvoicesScreen() {
	return (
		<PageScreen
			title="Invoices"
			headingLevel={2}
			subtitle="Everything billed this month"
			actions={
				<Button
					variant="solid"
					size="compact"
					render={<Link to="/invoices/new" />}
				>
					New invoice
				</Button>
			}
		>
			<InvoiceRows />
		</PageScreen>
	)
}
```

`subtitle` and `actions` are optional. The page spans the full canvas
width, so a screen with one action and a screen with four still line up.

Give every button in `actions` the compact size, `size="compact"`. It
is 32px tall, like the buttons of a WordPress page header, and the
title and its subtitle stay where every other page puts them.

Mount the screen on your plugin's route with `lazyRouteComponent`,
rather than importing it at the top of the routes file. The second
argument names the export to load.

```tsx
import { createRoute, lazyRouteComponent } from '@tanstack/react-router'

createRoute({
	getParentRoute: () => parent,
	path: '/invoices',
	component: lazyRouteComponent(() => import('./InvoicesScreen'), 'InvoicesScreen'),
})
```

Plugin screens load on demand. The browser fetches a screen's code the
first time a reader opens it, so the first paint stays small however
many plugins are installed. The bundle check,
`test/e2e/tests/bundle.spec.ts`, enforces this. It fails when the code
loaded before first paint grows past its ceiling, or carries a screen
it names. Name your screen there too.

When one page has sections that each need their own address, give it
tabs. The Users page does this for an account that manages users: the
Users tab lists the accounts and the API tokens tab lists the tokens.
Both screens keep the title Users and pass the same tabs, and each one
marks its own tab as current:

```tsx
import { PageScreen, PageTab, PageTabs } from '@alphone/frontend-sdk'
import { Link } from '@tanstack/react-router'

<PageScreen
	title="Invoices"
	headingLevel={2}
	tabs={
		<PageTabs label="Invoice sections">
			<PageTab
				render={<Link to="/invoices" activeOptions={{ exact: true }} />}
				current
			>
				Invoices
			</PageTab>
			<PageTab
				render={
					<Link to="/invoices/reminders" activeOptions={{ exact: true }} />
				}
			>
				Reminders
			</PageTab>
		</PageTabs>
	}
>
	<InvoiceRows />
</PageScreen>
```

Each screen keeps its own subtitle and its own buttons, so the title
stays the same while the sentence under it and the button beside it
follow the tab. `activeOptions={{ exact: true }}` keeps the router from
marking the first tab as current on the second tab's address.

Leave out the tabs a reader cannot open. An account that cannot open
Users finds API tokens in the menu where Users would be, the way
WordPress shows Profile in its place, and its tokens page reads API
tokens with no tabs at all.

Pass `aside` to show a second column beside the content, for the short
fields and facts of one record, such as its customer and its dates. What
grows, such as the lines of an invoice, stays in the content. On a narrow
page the aside moves under the content, and an aside that renders nothing
takes no room. `asideLabel` names the column for a screen reader, so pass it
with `aside`:

```tsx
<PageScreen
	title="Invoice 1042"
	headingLevel={2}
	aside={<InvoiceDetails />}
	asideLabel="Invoice details"
>
	<InvoiceLines />
</PageScreen>
```

A page with an `aside` keeps its title in view, and on a wide page each
column scrolls on its own. That works only with no element of your own
around `PageScreen`.

A plugin's contact panel always renders in the contact page's main column,
under the tasks. That column changes width with the screen, so let your
panel fill the room it gets.

A panel may load on demand too. Hand `contactPanels` a
`lazyRouteComponent`, as `plugins/fields/frontend/index.ts` does:

```ts
contactPanels: [{
	id: 'invoices',
	Panel: lazyRouteComponent(() => import('./ContactInvoicesPanel'), 'ContactInvoicesPanel'),
}],
```

The host starts loading every such panel as the contact page opens, so
its code arrives while the contact itself loads. Until it arrives, the
panel takes no room.

Every page opened from a list takes breadcrumbs, a create page too. They
are the links back to that list, and they sit before the title:

```tsx
import { PageBreadcrumb, PageBreadcrumbs, PageScreen } from '@alphone/frontend-sdk'
import { Link } from '@tanstack/react-router'

<PageScreen
	title="Invoice 1042"
	headingLevel={2}
	breadcrumbs={
		<PageBreadcrumbs label="Breadcrumbs">
			<PageBreadcrumb
				render={<Link to="/invoices" activeOptions={{ exact: true }} />}
			>
				Invoices
			</PageBreadcrumb>
		</PageBreadcrumbs>
	}
>
	<InvoiceLines />
</PageScreen>
```

`label` names the links for a screen reader, so translate it.
`activeOptions={{ exact: true }}` keeps the router from marking the link
as the current page, as it does for a tab. Leave out the page on screen,
since its title already names it. Beside breadcrumbs the title stays on one
line, so a long title ends in an ellipsis.

A record that is still loading, or that failed to load, keeps its
breadcrumbs and has no `aside`. Its title names the kind of record, such as
Invoice.

Put navigation and creation controls in `actions`. A form's submit
button belongs at the bottom of the form, not in the header.

A page with an `aside` is the exception. Its title stays in view, so it
takes one compact **Save**, last in `actions`, for the record's fields in
both columns:

```tsx
<Button
	type="submit"
	form={formId}
	size="compact"
	disabled={!changed || saving}
	loading={saving}
>
	Save
</Button>
```

`form` holds the `id` of the form the button submits, so Enter in a field
still saves. Name `saving` in `disabled` too, since `loading` alone does not
turn off a button that has `disabled`. A row added to a list keeps its own
button, and so does a contact panel, since a panel cannot reach the page
header.

Your screen renders in two shells without doing anything. On a wide
viewport it sits beside the sidebar. Below 1024px the sidebar
becomes a drawer behind a menu button. Below 782px the canvas meets the
screen edges, as a WordPress page does on a tablet, and below 640px it
pads tighter. Build one screen and check it at each of these sizes.

## Who is signed in

A screen reads the signed-in account with `useSession`. It answers the
account's id, email, name, and role, what that role may do, and the
roles it may give another account, or `null` when nobody is signed in.

Ask what the account may do, never which role it holds:

```tsx
import { MANAGE_USERS, can, useSession } from '@alphone/frontend-sdk'

export function InvoicesScreen() {
	const session = useSession()
	const manages = can(session, MANAGE_USERS)
	…
}
```

A deployment names its own roles, and a plugin may add more, so a
screen comparing `session.role` to `'admin'` breaks the moment somebody
installs a plugin that declares a role of its own. Asking `can` keeps
working, because the server answers what the role holds rather than
what it is called. A session with no answer holds nothing, so a screen
that cannot tell hides the control rather than offering it.

`can` comes from the shared accounts kit, `@gopherium/react-auth`, and
the SDK hands it on unchanged, so it takes the same two arguments in
every application built on that kit.

A route can name the capability a reader needs to open it:

```tsx
createRoute({
	getParentRoute: () => parent,
	path: '/invoices',
	staticData: { capability: 'invoices.manage_invoices' },
	component: lazyRouteComponent(() => import('./InvoicesScreen'), 'InvoicesScreen'),
})
```

The sidebar then leaves out a row pointing at that route, or at a route
nested under it, for a reader without the capability. A row may also
name a `capability` of its own, and the sidebar checks both. The core
keeps Users and Add User from an account without `manage_users` this
way, and sends that account to its home screen when it types their
address.

Sending a reader home covers only the core's own screens. Your plugin's
routes mount beside them, outside that check, so a reader who types
your route's address still reaches your screen. Keep that reader out
in the screen itself: ask `can`, and render a refusal in place of the
content when it says no. `renderPluginAt` mounts your routes the same
way, so a test of your refusal sees what a reader sees.

Hiding a control is presentation, not protection. The backend refuses
an operation the caller may not run whether or not your screen showed
the button. Hide the button so the screen only offers what it can
deliver, and let the backend do the refusing.

## The four states

A screen that loads data has four states, and the template has an answer
for each. Handle them in this order.

```tsx
if (invoices.isPending) {
	return <LoadingScreen label="Loading invoices…" />
}
if (invoices.isError) {
	return <ErrorNotice>Invoices could not be loaded.</ErrorNotice>
}
if (rows.length === 0) {
	return <ListEmpty icon={people} title="No invoices yet" actions={<Button onClick={addInvoice}>Add Invoice</Button>} />
}
return <InvoiceTable rows={rows} />
```

`LoadingScreen` announces its label politely. `ErrorNotice` announces
the failure as an alert, which plain copy in a `Text` never does.

`ListEmpty` draws the icon, the title, an optional `description` and
optional `actions` centred on the canvas, as WordPress draws an empty
list. Give `actions` the add button from your header, at its default
size. The title sits one level under the page title, so it never
competes with it. A list that a search or a filter emptied shows
`<ListNoResults />` instead, which you pick with `viewNarrows(view)`.

## Lists, tables, and paging

Give every list an accessible name, and render it through the design
system rather than a bare element.

```tsx
<Stack aria-label="Open invoices" render={<ul />}>
```

Tables use the `godmin-table` class so padding, borders, and header
weight match every other table. Each table sits in a
`godmin-table-scroll` region, so a wide table never drags the whole
page sideways.

```tsx
<div className="godmin-table-scroll" role="region" aria-label="Invoices" tabIndex={0}>
	<table className="godmin-table">
		<thead>
			<tr>
				<th scope="col">Invoice</th>
				<th scope="col">Total</th>
				<th scope="col" className="godmin-table__actions" />
			</tr>
		</thead>
		<tbody>
			<tr>
				<td>
					<div className="godmin-table__cell">{invoice.number}</div>
					<Text variant="body-sm">{invoice.note}</Text>
				</td>
				<td>
					<div className="godmin-table__cell">{invoice.total}</div>
				</td>
				<td className="godmin-table__actions">
					<div className="godmin-table__cell">
						<Button size="compact" onClick={send}>Send</Button>
					</div>
				</td>
			</tr>
		</tbody>
	</table>
</div>
```

The region needs all three attributes. The label says what the table
holds. `tabIndex` lets a keyboard reach the region. A table wider than
its region scrolls sideways inside it, on any page, and a keyboard user
scrolls it from there. Without `tabIndex` they cannot reach the columns
out of view at all.

A body cell sits at the top of its row, as in the WordPress list
table. Put the first line of each body cell in a `div` marked
`godmin-table__cell`. The box is as tall as a compact button and
centres its line, so the text stays level with the buttons of its row.
The box holds inline content, such as text, a link, a `code` or a
`Badge`, and not a paragraph. A line under it, such as a note, goes
after the box in its own element, such as a `Text`. Draw the buttons
of a row with `size="compact"`. A default button is taller than the
box, so the text beside it would sit higher than the button's label.

When the region is narrower than 640px, such as on a phone, each row
draws as an item of the DataViews list instead. The first cell and the
cell marked `godmin-table__actions` share the first line. The other
cells sit under them as small text, the column headers hide, and
nothing scrolls sideways. So keep the name of the record in the first
cell, and mark the cell of the row's buttons `godmin-table__actions`.
At 640px and wider the table keeps its columns.

The region measures its own width to pick a layout. Put it in a parent
that stretches it, such as a page section, a record page column or a
column `Stack` with no `align`. In a parent that sizes it to its
content, such as a row `Stack`, the region shrinks to almost nothing.

Form fields go inside `godmin-form`, which stacks them and keeps the
column readable. A form inside a dialog is `FormDialog` instead, or
`RenameDialog` for one new name, as [Dialogs](#dialogs) shows.

For a list read with `useConnection`, `LoadMore` renders the next page
button and hides itself once every page is loaded. When the list is
refetched, every loaded page is read again from the first, so the rows keep
their current order. If a few new rows push the last loaded rows down, the
reading goes on until it reaches them. If many new rows arrive at once, the
last loaded rows move behind the button instead.

```tsx
<LoadMore query={invoices}>Load more</LoadMore>
```

## Lists with DataViews

A short table you fill yourself fits the part above. A list a reader
searches, sorts, filters and pages through is a WordPress DataViews
list, like Contacts and Users. Import `DataViews`, its types and
`listDate` from `@alphone/frontend-sdk/dataviews`, and everything else
from `@alphone/frontend-sdk`, every dialog included.

### The view lives in the address

DataViews keeps the search, the sort, the filters, the page and the
page size in one object, the **view**. `useListView` keeps that view in
the address, so a reload, the back button or a shared link opens the
same list. Give the route `validateSearch: listSearch`. It keeps what a
list understands from the address and drops anything malformed.

```tsx
import { listSearch } from '@alphone/frontend-sdk'
import { createRoute, lazyRouteComponent } from '@tanstack/react-router'

createRoute({
	getParentRoute: () => parent,
	path: '/invoices',
	validateSearch: listSearch,
	component: lazyRouteComponent(() => import('./InvoicesScreen'), 'InvoicesScreen'),
})
```

The screen reads the page sizes from the admin settings, asks the
server for one page, and hands everything to `DataViews`:

```tsx
import {
	ErrorNotice,
	PageScreen,
	__,
	paginationOf,
	useAdminSettings,
	useGraphQuery,
	useListView,
	useServerPaging,
} from '@alphone/frontend-sdk'
import { DataViews } from '@alphone/frontend-sdk/dataviews'
import { useMemo } from 'react'

export function InvoicesScreen() {
	const { settings, failed } = useAdminSettings()
	const list = useListView({
		fields: ['status', 'issued'],
		titleField: 'number',
		sort: { field: 'issued', direction: 'desc' },
		perPage: settings?.listPageSize,
	})
	const sizing = settings === undefined && !failed
	const paging = useServerPaging(settings === undefined ? { page: list.view.page } : list.view)
	const [result] = useGraphQuery({
		query: invoicePageQuery,
		variables: { q: list.view.search || null, ...paging.window },
		pause: sizing,
		requestPolicy: 'cache-and-network',
	})
	const page = result.data?.invoicePage
	paging.record(page, result.operation?.variables.limit ?? null)
	const fields = useMemo(() => invoiceFields(), [])

	return (
		<PageScreen title={__('Invoices', 'acme-billing')} headingLevel={2} list>
			{result.error ? (
				<ErrorNotice>{__('Invoices could not be loaded.', 'acme-billing')}</ErrorNotice>
			) : (
				<DataViews
					data={page?.items ?? []}
					paginationInfo={paginationOf(page)}
					fields={fields}
					view={list.view}
					onChangeView={list.onChangeView}
					defaultLayouts={list.defaultLayouts}
					selection={list.selection}
					onChangeSelection={list.onChangeSelection}
					isLoading={result.fetching || (result.data === undefined && result.error === undefined)}
					searchLabel={__('Search invoices…', 'acme-billing')}
					config={settings === undefined ? undefined : { perPageSizes: settings.listPageSizes }}
				/>
			)}
		</PageScreen>
	)
}
```

`invoicePageQuery` is your plugin's own query. It takes `q`, `limit`
and `offset`, and answers `items`, `total` and `limit`.
`invoiceFields` returns your DataViews fields. Build them inside a
function, so their labels read the catalogue the reader loaded.

`useListView` takes what the list opens on. `fields` are the columns
beside the title, `titleField` names each row, and `sort` is the first
order. AlphOne lists open newest first. `perPage` comes from the admin
settings, so the operator picks one page size for every list. Hand
`view`, `onChangeView`, `defaultLayouts`, `selection` and
`onChangeSelection` straight to `DataViews`, as the example does.
Below 640px the hook lays the list out as a list instead of a table,
`phoneFields` picks what shows there beside the title, and `phone`
says the hook is in that layout.

`list` on `PageScreen` lets the list fill the page down to the bottom
edge. `isLoading` keeps the search box and the columns on screen while
a page or the settings are still on their way.

### Paging on the server

`useServerPaging` and `paginationOf` come from the shared admin kit,
and the SDK hands them on. `paging.window` holds the two numbers your
query sends: `limit`, how many rows to ask for, and `offset`, how many
to skip first. On page 3 with 20 rows a page, that is
`{ limit: 20, offset: 40 }`.

Your server answers a page with its rows, `total`, how many rows match
in all, and `limit`, the page size it really used. `paginationOf(page)`
turns that into the counts DataViews shows, and zero while the page is
on its way.

`paging.record(page, asked)` remembers the size the server used.
`asked` is the limit of the request that got this page, and urql keeps
it in `result.operation?.variables.limit`. A server may cut a large ask
down to a cap of its own. When it does, the next pages step by the size
it used, so no row goes missing or shows twice. An answer to an older
request, with another page size, never changes the step. When you know
the cap, pass it as the second argument, `useServerPaging(list.view, cap)`.

`pause` holds the request until the settings arrive, so the first
request already asks for the right size. When the settings cannot be
read, the screen hands `useServerPaging` only the page, even when the
address names a page size. The request then sends a `null` limit, and
the server picks the size.

### Reading fresh after a change

urql keeps every answer in a cache. With its default policy, a query
that mounts again shows the cached answer and asks the server nothing.
So when your New invoice screen adds a row and sends the reader back,
the list still shows the page from before. `graph.refetch` cannot help
there, because it only reruns the queries on screen at that moment.

Give every list that must show changes made somewhere else
`requestPolicy: 'cache-and-network'`, as the example does. It shows the
cached page at once, asks the server anyway, and swaps in the fresh
page when it arrives.

Each account that signs in gets a cache of its own. It starts empty, so
the first screen that account opens always asks the server.

### Types without an id

The cache files every answer by its type and its `id`. A type of yours
that has no `id`, such as `InvoicePage`, needs a key on `graphKeys`, or
the browser console warns `Invalid key` each time one arrives. Answer
`null` to keep the value inside the answer that holds it, or a field
that tells two of them apart:

```ts
graphKeys: {
	InvoicePage: () => null,
	InvoiceStatus: (data) => String(data.code),
},
```

The core keys its own types, such as `AdminSettings`. A plugin that keys
a type the core or another plugin already keys stops the app from
starting, with an error naming both.

`renderPluginAt` hands your plugin's keys to the fake graph client. A
test that builds a client itself passes the plugin too, with
`fakeGraphClient([plugin])`.

### Bulk actions

An action with `supportsBulk: true` runs on every row the reader
ticked. `useBulkRun` makes one call per row, all at once, marks the
list reloading as the first call that works ends, shows one toast in
the words you pass as `BulkWords`, then refreshes your list:

```tsx
import { __, _n, formatNumber, inbox, sprintf, useGraph } from '@alphone/frontend-sdk'
import type { BulkRun, BulkWords } from '@alphone/frontend-sdk'
import type { Action } from '@alphone/frontend-sdk/dataviews'
import { useMemo } from 'react'

const archiveWords: BulkWords<InvoiceRow> = {
	done: (done) =>
		sprintf(_n('%s invoice archived.', '%s invoices archived.', done, 'acme-billing'), formatNumber(done)),
	failed: (failures) =>
		sprintf(
			_n('%s invoice could not be archived.', '%s invoices could not be archived.', failures.length, 'acme-billing'),
			formatNumber(failures.length),
		),
}

export function useArchiveAction(bulk: BulkRun<InvoiceRow>): Action<InvoiceRow> {
	const graph = useGraph()
	return useMemo(
		() => ({
			id: 'archive',
			label: __('Archive', 'acme-billing'),
			icon: inbox,
			supportsBulk: true,
			callback: (invoices) =>
				bulk(
					invoices,
					(invoice) => graph.client.mutation(archiveInvoiceMutation, { id: invoice.id }).toPromise(),
					archiveWords,
				),
		}),
		[graph, bulk],
	)
}
```

The screen holds the run and the refresh, and hands the action its
`bulk`:

```tsx
const { run, refreshing, region } = useListRun('invoices', list.onChangeSelection)
const refresh = useCallback(
	() => graph.client.query(invoicePageQuery, pageVariables, { requestPolicy: 'network-only' }).toPromise(),
	[graph, pageVariables],
)
const archive = useArchiveAction(useBulkRun<InvoiceRow>(run, refresh))
```

A call fails when it throws, when its promise rejects, or when it
answers with its `error` set, so an urql result with an error counts
as a failure. One bad row never hides the others. `useBulkRun` shows
the toast as the last call ends and only then calls `refresh`, so the
old rows stay on screen until it ends. Add `refreshing` to the list's
`isLoading` so those rows grey out. `refresh` must return a promise,
since the list stays marked as reloading until that promise settles.
`graph.refetch` returns nothing, so read the list again with
`graph.client.query` as above. Wrap `refresh` in `useCallback`, since
a new `refresh` on every render hands your actions a new run on every
render too.

The toast holds your `failed` words when any row failed, and your
`done` words when every row worked. Nothing shows above the list.
`done` gets the one row when only one was asked, so you can name it,
and `failed` gets each failure with its error and how many rows were
asked. `useListRun` keeps the ticked rows in step with the list.

Pass its `region` to `PageScreen` as `listRef`, with a `listLabel`, so
a keyboard reader keeps their place. As the list starts to reload, the
region takes a focus from the table or the footer. After a run started
from the keyboard it draws the focus ring. Once the new rows land the
focus goes back to the control the reader pressed, then to the same item
wherever its row now sits, then to the first control of the row in its
place. It stays on the region when that control is a primary action, so
a one click action never takes a second Enter meant for another row.

To let it find the same item, mark the title cell of the list:

```tsx
render: ({ item }) => <span data-item-id={item.id}>{item.name}</span>
```

DataViews draws each bulk action by its label. At 782px wide and below,
it puts them all in one **Actions** menu. When one row was picked and
it failed, show the server's reason instead of a count.
`failures[0].error` is `unknown`, so cast it to `GraphFailure` and pass
it through `graphError` and `validationMessage`, as in
[Failures in forms](#failures-in-forms).

### Dialogs

A dialog that stays open says inside it why a write failed. A dialog
that closes says how its write went in a toast at the bottom of the
screen, as WordPress does, with `useToaster`. Every dialog below comes
from `@alphone/frontend-sdk`.

A dialog waits on its write only up to a time limit. The limit is the
server's `ALPHONE_DIALOG_WRITE_TIMEOUT` setting, as
[Configuration](/self-hosting/configuration/) says. `onConfirm` and
`onSubmit` get the dialog's signal as their last argument.
`RenameDialog` hands it to `onSubmit` after the name. A signal is an
`AbortSignal`, which is how the browser tells a request to stop, and
it fires once the limit runs out.

Send every write a dialog waits on through `dialogWrite` from
`@alphone/frontend-sdk`, never through
`graph.client.mutation(...).toPromise()`. It takes the client, the
mutation, its variables and the signal:

```tsx
dialogWrite(graph.client, deleteInvoiceMutation, { id: invoice.id }, signal)
```

It answers what the urql call answers, its `error` included, so you
turn a failed write into words as before. Once the signal fires, it
stops the request and rejects. When the handler returns that write
itself, the dialog then shows WordPress's words for a write with no
answer, "Could not get a valid response from the server." When the
browser is offline at that moment, it says "Unable to connect. Please
check your Internet connection." instead. A run of `useBulkRun` or
`useDialogRun` counts a row still writing as failed, and says so in
your `failed` words.

The server may still finish a write it got before the request
stopped, so a second send after the limit ran out can make the change
twice. TypeScript does not point at a handler that leaves the signal
out, so check each dialog yourself.

An action that cannot be undone asks first in `ConfirmDialog`. For a
row action or a bulk action, give the action a `callback` that stores
the picked rows, and render the confirm from your screen while it
asks. `title` is the question. Name one row in it and count several.
`description` says what will happen, and `intent="irreversible"` draws
the confirm button red. Its `onConfirm` returns the run, so the
confirm stays open with a spinner until the toast shows. Each call of
the run hands the signal to its write:

```tsx
const [deleting, setDeleting] = useState<InvoiceRow[]>()
const actions = useMemo<Action<InvoiceRow>[]>(
	() => [
		{
			id: 'delete',
			label: __('Delete', 'acme-billing'),
			icon: trash,
			supportsBulk: true,
			callback: setDeleting,
		},
	],
	[],
)
const deleteOne = (signal: AbortSignal) => (invoice: InvoiceRow) =>
	dialogWrite(graph.client, deleteInvoiceMutation, { id: invoice.id }, signal)
const one = (invoice: InvoiceRow) =>
	sprintf(__('Delete "%(name)s"?', 'acme-billing'), { name: invoice.name })
const many = (count: number) =>
	sprintf(
		_n('Delete %s invoice?', 'Delete %s invoices?', count, 'acme-billing'),
		formatNumber(count),
	)

{deleting ? (
	<ConfirmDialog
		intent="irreversible"
		title={deleting.length === 1 ? one(deleting[0]) : many(deleting.length)}
		description={__('This action cannot be undone.', 'acme-billing')}
		confirmLabel={__('Delete', 'acme-billing')}
		cancelLabel={__('Cancel', 'acme-billing')}
		onConfirm={(signal) => bulk(deleting, deleteOne(signal), deleteWords)}
		onClose={() => setDeleting(undefined)}
	/>
) : null}
```

Write an action's label with no ellipsis, as **Delete** is, even when
the action opens a dialog. DataViews asks for labels this way. While
the run goes, Cancel and Escape close nothing. `onClose` runs once the
card has left the page, and that is where you take the dialog away.

A rename asks in `RenameDialog`, through `useDialogRun`, which takes
what `useBulkRun` takes. The dialog stays open while it saves, with a
spinner on Save and the field read only. It closes when the write
works, with your `done` toast. When the write fails, it stays open
with your `failed` words inside and the typed name kept.

The write that `useDialogRun` hands out takes the signal as its fourth
argument, as `write(items, call, words, signal)`. The run does not
hand the signal to `call`, so pass it on to each write yourself:

```tsx
const write = useDialogRun<InvoiceRow>(run, refresh)
const [renaming, setRenaming] = useState<InvoiceRow>()
const renameTo = (name: string, signal: AbortSignal) => (invoice: InvoiceRow) =>
	dialogWrite(graph.client, renameInvoiceMutation, { id: invoice.id, name }, signal)

{renaming ? (
	<RenameDialog
		title={__('Rename', 'acme-billing')}
		name={renaming.name}
		fieldLabel={__('Name', 'acme-billing')}
		submitLabel={__('Save', 'acme-billing')}
		cancelLabel={__('Cancel', 'acme-billing')}
		onSubmit={(name, signal) =>
			write([renaming], renameTo(name, signal), renameWords, signal)
		}
		onClose={() => setRenaming(undefined)}
	/>
) : null}
```

Save stays on with a blank name, as in WordPress, and the browser
rejects a blank name, or one of spaces only, before anything is sent.
A form over one row or over the picked rows is a `FormDialog` that
writes through `useDialogRun` in the same way. Its `onSubmit` gets the
signal as its only argument:

```tsx
onSubmit={(signal) =>
	write(
		editing,
		(invoice) => dialogWrite(graph.client, dueDateMutation, { id: invoice.id, dueDate }, signal),
		dueDateWords,
		signal,
	)
}
```

Over one row, a failed write keeps the dialog open with your `failed`
words inside. Over several rows, the dialog closes and one toast says
how the run went.

`dialogWrite` answers a failed write with its `error` set rather than
rejecting, as an urql call does. `useDialogRun` counts that answer as
a failure, so no `.catch` is needed.

A confirm outside a list, such as a button in your screen's header,
is the same `ConfirmDialog`. It is a card in the middle of the screen
at every width, a phone included, and it always needs a `title`. What
`onConfirm` returns decides when it goes. A plain function that
returns nothing closes it at once while your write runs, so show the
work on the button that opened it. Return the write's promise to keep
it open with a spinner until the write ends or the time limit runs
out. Answer `{ error: words }` to keep it open and show the words
inside, when trying again is the next step. An `async` function always
returns a promise, so keep `onConfirm` plain when the dialog should
close at once.

`dialogWrite` answers a failed write with an object in `error`, and
the dialog reads only words there. So turn that answer into words,
or the dialog closes as if the write worked:

```tsx
const failed = __('Could not delete the invoice.', 'acme-billing')

<ConfirmDialog
	title={__('Delete this invoice?', 'acme-billing')}
	description={__('This action cannot be undone.', 'acme-billing')}
	intent="irreversible"
	confirmLabel={__('Delete', 'acme-billing')}
	cancelLabel={__('Cancel', 'acme-billing')}
	onConfirm={(signal) =>
		dialogWrite(graph.client, deleteInvoiceMutation, { id: invoice.id }, signal).then((result) =>
			result.error ? { error: writeFailure(failed, result.error) } : undefined,
		)
	}
	onClose={() => setDeleting(undefined)}
/>
```

The `.then` reads only an answer. Once the signal fires, the
rejection passes it by and the dialog shows WordPress's words. Add no
`.catch` that turns that rejection into `{ error }`, or your words
hide theirs.

`FormDialog` asks for a form from a button, as WordPress opens Add
Page: a title beside a close icon, the first field focused, and Cancel
and the send button at the end. It closes itself when its write ends.
`onSubmit` gets the signal and answers in the same way as
`onConfirm`. So hand the signal to `dialogWrite`, and answer
`{ error }` to keep the dialog open and say why, with what the reader
typed still there.

A page that holds unsaved work, such as a form the reader is filling
in, asks before a move in the app leaves it. Render
`UnsavedChangesDialog` with `when` set while the work is unsaved, and
your words for the question:

```tsx
<UnsavedChangesDialog
	when={changed}
	title={__('Discard changes?', 'acme-billing')}
	description={__('The invoice is not saved.', 'acme-billing')}
	discardLabel={__('Discard', 'acme-billing')}
	keepLabel={__('Keep editing', 'acme-billing')}
/>
```

It asks when the reader follows a link, uses the sidebar, or presses
Back or Forward. A reload or a closed tab still shows the browser's
own box. A handler that saves
and then leaves in one go passes `ignoreBlocker: true` to its
`navigate`, since the page counts as unsaved until the save has drawn.

### Channel names

A list that shows a channel, such as the WhatsApp number a record uses,
names it with `channelName`. It answers the name the core or any plugin
gives that channel, in the reader's language, or the channel itself
when nobody names it. Draw it as an outline badge, as Contacts does:

```tsx
import { Badge, channelName } from '@alphone/frontend-sdk'

<Badge intent="none">{channelName(identity.channel)}</Badge>
```

Call it while you render, never once when the file loads, so it
follows the language the reader picks.

Your plugin names its own channels on `channels`. Write each label as a
getter, so it reads the catalogue every time it is asked.
`plugins/whatsapp/frontend/index.ts` does this:

```ts
channels: [
	{
		value: 'whatsapp',
		get label() {
			return _x('WhatsApp', 'contact channel', 'alphone-whatsapp')
		},
	},
],
```

### Token areas

An API token is granted areas, such as `tasks:read`. Your plugin offers
its own area on the New token screen with `tokenAreas`. The value is the
`area` your schema's `@scope` names, and the label is how it reads in a
sentence, so write it as a getter too:

```ts
tokenAreas: [
	{
		value: 'invoices',
		get label() {
			return _x('invoices', 'token area', 'alphone-invoices')
		},
		get writeDescription() {
			return __('Reads invoices. Also voids an invoice. Sends no invoice.', 'alphone-invoices')
		},
	},
],
```

The optional `writeDescription` is the line under the area's Write box,
and it says what Write does in your area.

The screen lists the core areas and every plugin's in order of their
values. An area the core or another plugin already offers stops the
screen with an error naming both.

### Testing a list screen

`renderPluginAt` from `@alphone/frontend-sdk/testing` mounts your
plugin at an address inside the admin's real sidebar, with Back, the
focus moves and the phone drawer, under a router and a toaster, with a
fake graph client. Pass `session` to sign somebody in. A row that
names a `capability` shows only when that account carries it. The session goes straight
into the test's query cache, so no session request goes out. Without
one, the host asks the test server for the session, as the app does,
so your test answers it. Answer `/api/auth/session` with an account to
sign somebody in, or with a 401 for nobody. With nobody signed in, a
screen behind a capability shows its refusal.

```tsx
import { adminSession, graphql, HttpResponse, renderPluginAt, server } from '@alphone/frontend-sdk/testing'
import { screen } from '@testing-library/react'
import { expect, test } from 'vitest'

import { plugin } from '../index'

const managing = { ...adminSession, capabilities: ['invoices.manage_invoices'] }

const invoicePage = {
	__typename: 'InvoicePage',
	items: [{ __typename: 'Invoice', id: '019f5a00-0000-7000-8000-0000000000b1', number: 'INV-0001' }],
	total: 1,
	limit: 20,
}

test('lists the invoices the server answers', async () => {
	server.use(graphql.query('InvoicePage', () => HttpResponse.json({ data: { invoicePage } })))
	renderPluginAt(plugin, '/invoices', { session: managing })

	expect(await screen.findByText('INV-0001')).toBeInTheDocument()
})
```

The test setup serves the admin settings, with 20 rows a page.
`paging([1, 2], 1)` serves smaller sizes, so two rows are enough to
reach page 2.

`renderPluginAt` hands back the fake graph client and the router.
`graph.refetch` is a spy, so a test can check that an action refreshed
the list with `expect(graph.refetch).toHaveBeenCalledWith(['InvoicePage'])`.
The router tells where the screen went, in `router.state.location`.

## Dates, times, numbers and money

Write every date, time, number and amount with the SDK formatters, never
with `toLocaleString` or a locale of your own. They follow the locale the
operator names in `ALPHONE_FORMAT_LOCALE`, whatever language the reader
picked, so every screen agrees.

```tsx
import {
	formatDate,
	formatList,
	formatMoney,
	formatNumber,
	formatTime,
	formatWeekday,
} from '@alphone/frontend-sdk'

formatDate(invoice.issuedOn)
formatTime(message.sentAt)
formatNumber(invoice.lines)
formatMoney(invoice.total, 'EUR')
formatWeekday(invoice.issuedOn)
formatList(['Birth date', 'Shoe size'])
```

With the default locale these read 30/09/2026, 09:05, 1.234 and
1.234,56 €. `formatWeekday` names the day in the reader's language, for a
heading such as Wednesday, 30/09/2026. `formatList` joins words the way the
reader's language writes a list, such as Birth date, Shoe size. Keep dates
ISO in your queries, mutations and events, and format them only where a
reader sees them.

`formatDate` and `formatWeekday` keep a bare calendar day such as
`2026-09-30` on the day it names. In the same way, `formatTime` takes a
bare wall clock time such as `09:30`, the way a task's time of day travels,
and writes it as it stands: 09:30 with the default locale, or 09:30 AM in
`en-US`, whatever time zone the reader is in. A full moment such as
`message.sentAt` is shown on the reader's clock.

`formatDate` also takes the parts of the date to show, the options
`Intl.DateTimeFormat` reads, such as
`formatDate(invoice.issuedOn, { month: 'long', year: 'numeric' })` for a
month heading. An empty string writes nothing, so
`formatDate(invoice.paidOn ?? '')` stays blank for an invoice not paid yet.

A date column in a DataViews list is the one exception. Render it with
`listDate`, which writes the date and time the way WordPress lists do, in
the reader's language, such as Oct 13, 2026 5:04 pm:

```tsx
import { listDate } from '@alphone/frontend-sdk/dataviews'

render: ({ item }) => listDate(item.createdAt)
```

## Failures in forms

A failed write shows in one of two places, and the reader's edits stay in
the fields.

A server message about one field goes under that field, and the focus moves
there. `useRefusedField` holds the message and `ValidatedInputControl` shows
it. It goes as the reader edits.

Anything else goes in a toast. `writeFailure` returns what failed, then the
server's words when it named a reason or rejected the values. A form in a
dialog that stays open shows it inside the dialog instead, as
[Dialogs](#dialogs) says.

```tsx
const form = useRef<HTMLFormElement>(null)
const refusedName = useRefusedField(form, name)

const save = async () => {
	const result = await runSave({ name })
	if (!result.data && !refusedName.refuse(result.error, name)) {
		toaster.show(writeFailure(__('Could not save the invoice.', 'acme-billing'), result.error))
	}
}

<form ref={form}>
	<ValidatedInputControl
		label={__('Name', 'acme-billing')}
		value={name}
		customValidity={refusedName.validity}
		onChange={(event) => setName(event.target.value)}
	/>
</form>
```

`refuse` answers false when the failure has no words to show, or when the
field no longer holds what was sent, so the toast covers those cases too.

A field holding a message blocks its form until the message goes. When another
control changes what the server would answer, pass both to `useRefusedField`,
such as `${channel}:${value}`, and hand `refuse` the same string.

A screen that reads again after a failed write passes what the read holds to
`useLastAnswer`, so a read that fails too leaves the form and its edits on
screen. The screen still shows `ErrorNotice` for that read, and frees anything
that was waiting for it.

A check before sending stays under its field. `ErrorNotice` is for a failed
load. A failure in a dialog that stays open shows inside it, and a run over
list rows says how it went in a toast, as [Dialogs](#dialogs) says.

`validationMessage` shows a backend validation message verbatim and falls back
to your own copy for anything else. Throw `ValidationError` from your API layer
when the backend rejects the input, and anything else for a genuine failure.

A reason the server stores beside a record, such as the reason on an import
row, arrives as a `code` and its `meta`, or as null when the record needs
none. `reasonText` turns it into a sentence from a template map of your own,
writes every number in the format locale, and shows the fallback when no
template fits. Build the map inside a function, so every call reads the
catalogue the reader loaded.

```tsx
import { __, reasonText } from '@alphone/frontend-sdk'

function invoiceReasons(): Record<string, string> {
	return {
		invoice_late: __('Paid %(days)s days late.', 'acme-billing'),
	}
}

const shown =
	invoice.reason === null ? '' : reasonText(invoice.reason, invoiceReasons(), invoice.reason.code)
```

## Threads, recipients and attachments

A screen that shows a thread, one message under another, folds each
message in a `CollapsibleCard`. Wrap the header in a heading with
`render`, so a screen reader finds each message by its sender.
`defaultOpen` opens a card from the start, such as the newest message.

```tsx
import { Card, CollapsibleCard, Text } from '@alphone/frontend-sdk'

<CollapsibleCard.Root defaultOpen>
	<CollapsibleCard.Header render={<h3 />}>
		<Card.Title>Maria Perez</Card.Title>
	</CollapsibleCard.Header>
	<CollapsibleCard.Content>
		<Text>Can we meet on Monday?</Text>
	</CollapsibleCard.Content>
</CollapsibleCard.Root>
```

The whole header opens and closes the card, so keep buttons and links
out of it.

`FormTokenField` shows each address a reader types as a chip with a
button that removes it. `DropZone` takes the files a reader drops on
the element that holds it. Give that element `position: relative`,
because the zone stretches over its nearest positioned ancestor.
Import both from `@alphone/frontend-sdk/components`:

```tsx
import { DropZone, FormTokenField } from '@alphone/frontend-sdk/components'
```

That entry brings the WordPress components stylesheet these parts
need, the way `@alphone/frontend-sdk/dataviews` does, so the screens
that use neither never load it. To pick files with a button, use
`FileButton` from `@alphone/frontend-sdk`, as the Upload button on the
Import screen does.

`FormTokenField` and `DropZone` draw a few words of their own in
English. Pass `label`, `help` and `messages` to `FormTokenField`, and
`label` to `DropZone`, from your plugin's own text domain:

```tsx
import { __ } from '@alphone/frontend-sdk'

<FormTokenField
	label={__('To', 'acme-mail')}
	help={__('Separate addresses with commas or the Enter key.', 'acme-mail')}
	messages={{
		added: __('Address added.', 'acme-mail'),
		removed: __('Address removed.', 'acme-mail'),
		remove: __('Remove address', 'acme-mail'),
		__experimentalInvalid: __('Invalid address', 'acme-mail'),
	}}
	value={recipients}
	onChange={setRecipients}
/>
```

A chip's place in the list, the count of suggestions found and the
note that none matched stay in English.

The SDK also hands on the WordPress icons these screens draw:
`envelope` for mail, `commentReplyLink` for a reply, `lock` for an
encrypted message and `file` for an attachment. A text toolbar draws
`formatBold`, `formatItalic`, `link`, `formatListBullets`,
`formatListNumbered`, `quote` and `inlineImage`.

## Full bleed screens

Most screens sit on a padded canvas. A screen that fills its canvas edge
to edge, like a chat thread, opts out through its route.

```tsx
createRoute({
	getParentRoute: () => parent,
	path: 'threads/$threadId',
	component: lazyRouteComponent(() => import('./ThreadScreen'), 'ThreadScreen'),
	staticData: { canvas: 'bleed' },
})
```

`bleed` removes the canvas padding and stops the canvas scrolling, so
the screen owns its own scrolling region. `padded` is the default and
never needs declaring.

A bleed screen builds its own chrome, so it uses `PageTitle` directly
instead of `PageScreen`. Every route needs exactly one second level
heading, including a bleed one.

```tsx
<header className="alphone-thread__header">
	<PageTitle variant="heading-md" level={2}>{contactName}</PageTitle>
</header>
```

## Sidebar rows and sub menus

A plugin adds rows to the sidebar through `nav`, after the core rows.
Each row is a `MenuItem`: the `label` it shows, the address `to`, and
an optional `icon`, `capability`, `count` and `fallback`. A `count` is
`{ value, label }`, the number drawn at the end of the row and the
sentence a screen reader reads in its place. A `fallback` is the
row shown in its place to a reader who cannot open it, the way the core
shows API tokens in place of Users.

```ts
nav: [{
	get label() {
		return _x('Invoices', 'admin section', 'alphone-invoices')
	},
	to: '/invoices',
	icon: list,
}],
```

The SDK passes on a few icons from `@wordpress/icons`: `calendar`,
`check`, `chevronLeft`, `chevronRight`, `close`, `comment`,
`commentAuthorAvatar`, `inbox`, `institution`, `key`, `language`,
`list`, `menu`, `moveTo`, `notAllowed`, `pencil`, `people`, `trash` and
`upload`. The core rows carry `calendar` for Tasks,
`commentAuthorAvatar` for Contacts, `people` for Users and `language`
for Language. Fields carries `list`, Import carries `upload` and
WhatsApp carries `comment`, and each empty list draws the icon of its
row. Rows that list like items, such as the WhatsApp conversations,
each carry the same icon, so their labels line up under the sub menu
title.

A row can open a sub menu of its own, as the Site Editor's Navigation
row opens the list of menus. Give the route a `staticData.Sidebar`
component that draws a `NavigationScreen`. The admin swaps the main
menu for it, slides it in, and puts Back at its top, which leads to the
day of tasks. Put its rows in a `NavigationList`, each one a
`NavigationItem`. A sub menu on a parent route stays while the reader
moves between the screens under it, as the WhatsApp conversations stay
beside the thread a reader opens:

```tsx
import { NavigationItem, NavigationList, NavigationScreen } from '@alphone/frontend-sdk'

export function InvoicesSidebar() {
	return (
		<NavigationScreen title="Invoices">
			<NavigationList>
				<NavigationItem to="/invoices/drafts">Drafts</NavigationItem>
				<NavigationItem to="/invoices/paid">Paid</NavigationItem>
			</NavigationList>
		</NavigationScreen>
	)
}
```

Leave out the screen's `back`, so Back goes where the core's own sub
menus go. `NavigationList` also takes `items`, rows as `MenuItem` data,
and hides a row whose `capability` the reader lacks. A `NavigationItem`
takes the router's `Link` props, such as `params`, and hides itself
when its address opens a route the reader may not open.
`DrilldownItem` draws a row that opens a further sub menu, with a
chevron. `MenuCount` draws a count. Pass it as the `suffix` of a
`NavigationItem` or a `DrilldownItem`, for example
`suffix={<MenuCount count={3} label="3 drafts" />}`.

A sub menu may load on demand as a `lazyRouteComponent`. Give the
section route a `loader` that preloads it, as
`plugins/whatsapp/frontend/routes.tsx` does:

```tsx
import { createRoute, lazyRouteComponent } from '@tanstack/react-router'
import type { AnyRoute } from '@tanstack/react-router'

export function routes(parent: AnyRoute) {
	const Sidebar = lazyRouteComponent(() => import('./InvoicesSidebar'), 'InvoicesSidebar')
	return [
		createRoute({
			getParentRoute: () => parent,
			path: '/invoices',
			staticData: { Sidebar },
			loader: () => Sidebar.preload?.(),
		}),
	]
}
```

The router waits for the loader before it shows the section, so the
sidebar swaps the main menu for your sub menu in one step. Without the
loader, the sidebar shows no menu for a moment while the code arrives.

**To move** a plugin that still uses `NavItem` or
`SidebarNavigationScreen`:

1. Replace the type `NavItem` with `MenuItem` from
   `@alphone/frontend-sdk`. `NavItem` is gone, with no alias.
2. Replace `SidebarNavigationScreen` with `NavigationScreen` and drop
   `backTo`, so Back goes home. Draw the sub menu's rows in a
   `NavigationList`, so they look like every sidebar row and mark the
   current one.
3. Pass `headingLevel={2}` to every `PageScreen` and `level={2}` to a
   `PageTitle`, and draw an empty list with `ListEmpty`.
4. Stop importing `useCanvas` and `useFrameLocation`. Only the admin's
   frame reads them now, so the SDK no longer passes them on.
5. In every test through `renderPluginAt`, pass `session` or serve
   `/api/auth/session`, since the test host now reads the session to
   draw the sidebar and hides a row the reader cannot open. Find the
   sidebar with `getByRole('region', { name: 'Navigation' })`, and look
   for the page title at heading level 2, since the sidebar title is the
   only first level heading.

## Hiding your screens in a workspace

Some screens have nothing to show until the workspace holds something.
The Email row shows only once the workspace holds a mailbox, for
example. A plugin does the same on its Go side, with the one method of
`sdk.ScreenGate`:

```go
var _ sdk.ScreenGate = (*Plugin)(nil)

func (p *Plugin) ShowsScreens(ctx context.Context) (bool, error) {
	return p.holdsAccount(ctx, sdk.TenantOrDefault(ctx))
}
```

`ctx` carries the caller's workspace, and `sdk.TenantOrDefault` reads
it. Answer `true` to show your screens and `false` to hide them. A
plugin without the method always shows them.

The admin asks every plugin once as it opens and again after each
sign-in, through the
[`hiddenPlugins`](/reference/graphql-api/#hidden-plugins) field. It
then leaves out the sidebar rows and the contact panels of each plugin
that answered `false`. So a reader sees your row appear the next time
they sign in or reload the admin. Your routes stay in place, and a
reader who types the address still reaches your screen. Draw an empty
state there with `ListEmpty` that says what is missing, as the email
inbox says "No mailbox is connected".

An answer that fails, panics or comes too late keeps your screens shown,
so a plugin with a bug never takes a row away. All plugins share one
time limit for their answers, `ALPHONE_PLUGIN_SCREENS_BUDGET`, 50ms by
default, see [Plugin hooks](/self-hosting/configuration/#plugin-hooks).
Keep the answer to one short read, such as one row of the database.
`serve` logs each answer that failed, panicked or came too late on one
line, `plugin hook failed`, which names your plugin and the hook
`admin.screens`.

## Links under the account panel

A plugin adds links under the account panel, at the foot of the
sidebar, through `accountLinks`. Use them for addresses outside the
admin, such as a help page or a support chat. A screen of your own
still goes in `nav`. Each link is an `AccountLink`: the `label` it
shows, the address `href`, and an optional `capability` and
`openInNewTab`.

```ts
accountLinks: [{
	get label() {
		return __('Help center', 'alphone-help')
	},
	href: 'https://help.example.com/',
	capability: MANAGE_USERS,
	openInNewTab: true,
}],
```

The admin shows a link only to a signed-in reader, and only to one
holding its `capability` when it names one. The links keep the order
of the plugins and the order each plugin gives them. Every link sends
no referrer. A link with `openInNewTab` opens in a new tab and shows
an arrow, and its name tells a screen reader it opens in a new tab.
Leave it out for a `mailto:` address, which opens the reader's mail
program instead.

## What not to do

Almost all of these fail the test suite rather than reaching a user. The
last one is a convention a reviewer will hold you to.

| Do not | Do instead | Caught by |
| ------ | ---------- | --------- |
| Write a raw `<h1>` or `<h2>` | `PageScreen title`, `PageTitle`, or a design system heading | source invariant |
| Leave a route without a page title | Give every route exactly one second level heading | rendered outline test |
| Add a route without listing it in the outline test | List it | rendered outline test |
| Draw an empty list with `EmptyState` | `ListEmpty` | source invariant |
| Leave a table outside the scrolling region | Wrap it in `godmin-table-scroll` | source invariant |
| Let a screen spill sideways on a phone | Keep it inside the canvas | end to end fit sweep |
| Put failure copy in a plain `Text` | `ErrorNotice` for a failed load, `useRefusedField` for a message about one field, the dialog itself for a failure in a dialog that stays open, a toast for any other failed write | screen tests assert the alert role, the field's description, the dialog's words or the toast |
| Cap the page width yourself | Let the page span the canvas | end to end geometry test |
| Hand roll a load more button | `LoadMore` | convention |

Three kinds of test carry these rules.

The **source invariant** reads every screen file and rejects the
patterns above, so a raw heading fails before anything renders.

The **rendered outline test** mounts every route, counts the second
level headings on the canvas and checks that the sidebar holds the only
first level one, so a screen that renders no title fails even though
its source looks clean. It also compares its route list against the router,
so adding a route without covering it fails too.

**Screen tests** find the error state by its alert role rather than by
its text, so replacing an announced failure with silent copy breaks
them.

The **fit sweep** seeds long names and an unbreakable word, then visits
every route on a phone sized viewport and names whatever spills past the
canvas. Content is allowed to be wider than the screen only when it
scrolls inside its own container.
