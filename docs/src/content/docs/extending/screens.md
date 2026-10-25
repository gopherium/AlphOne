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

Pass `aside` to show a second column beside the content, for details that
sit next to the main work. On a wide page it starts 320px wide. On a narrow
page it moves under the content, and an aside that renders nothing takes no
room.

A plugin's contact panel always renders in the contact page's aside, so
design it for a 320px column.

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

Put navigation and creation controls in `actions`. A form's submit
button belongs at the bottom of the form, not in the header.

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
keeps Users and New user from an account without `manage_users` this
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
	return <Text role="status">Loading invoices…</Text>
}
if (invoices.isError) {
	return <ErrorNotice>Invoices could not be loaded.</ErrorNotice>
}
if (rows.length === 0) {
	return <ListEmpty icon={people} title="No invoices yet." hint="Add one with New invoice." />
}
return <InvoiceTable rows={rows} />
```

`Text role="status"` announces loading politely. `ErrorNotice` announces
the failure as an alert, which plain copy in a `Text` never does.

`ListEmpty` draws the icon, the title and the hint centred on the
canvas, as WordPress draws an empty list. Its title sits one level
under the page title, so it never competes with it. Leave out `hint`
when a search or a filter emptied the list.

## Lists, tables, and paging

Give every list an accessible name, and render it through the design
system rather than a bare element.

```tsx
<Stack aria-label="Open invoices" render={<ul />}>
```

Tables use the `godmin-table` class so padding, borders, and header
weight match every other table, and they sit inside a scrolling region
so a phone scrolls the columns rather than the whole page.

```tsx
<div className="godmin-table-scroll" role="region" aria-label="Invoices" tabIndex={0}>
	<table className="godmin-table">…</table>
</div>
```

The region needs all three attributes. `tabIndex` lets a keyboard reach
the columns that scrolled out of view, and the label says what they
belong to.

Form fields go inside `godmin-form`, which stacks them and keeps the
column readable. A form inside a dialog is `FormBody` instead, as
[Dialogs](#dialogs) shows.

For a list read with `useConnection`, `LoadMore` renders the next page
button and hides itself once every page is loaded. When the list is
refetched, every loaded page is read again from the first, so the rows keep
their current order.

```tsx
<LoadMore query={invoices}>Load more</LoadMore>
```

## Lists with DataViews

A short table you fill yourself fits the part above. A list a reader
searches, sorts, filters and pages through is a WordPress DataViews
list, like Contacts and Users. Import `DataViews`, its types and
`listDate` from `@alphone/frontend-sdk/dataviews`, `ConfirmDialog` and
`FormDialog` from `@alphone/frontend-sdk/dialogs`, and everything else
from `@alphone/frontend-sdk`.

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
					isLoading={result.fetching || sizing}
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
settings, so the operator picks one page size for every list. Hand the
five parts the hook answers straight to `DataViews`. Below 640px the
hook lays the list out as a list instead of a table, and `phoneFields`
picks what shows there beside the title.

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
ticked. `useBulkRun` makes one call per row, all at once, refreshes
your list and shows one toast in the words you pass as `BulkWords`:

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
const { run } = useListRun('invoices', list.onChangeSelection)
const refresh = useCallback(
	() => graph.client.query(invoicePageQuery, pageVariables, { requestPolicy: 'network-only' }).toPromise(),
	[graph, pageVariables],
)
const archive = useArchiveAction(useBulkRun<InvoiceRow>(run, refresh))
```

A call fails when it throws, when its promise rejects, or when it
answers with its `error` set, so an urql result with an error counts
as a failure. One bad row never hides the others. `useBulkRun` waits
for `refresh` before the toast, because DataViews never tells the
screen an action finished, so `refresh` must return a promise.
`graph.refetch` returns nothing, so read the list again with
`graph.client.query` as above. Wrap `refresh` in `useCallback`. When
it changes during a run, the bulk bar draws the confirm again and the
run loses its busy state.

The toast holds your `failed` words when any row failed, and your
`done` words when every row worked. Nothing shows above the list.
`done` gets the one row when only one was asked, so you can name it,
and `failed` gets each failure with its error and how many rows were
asked. `useListRun` keeps the ticked rows in step with the list. It
moves the focus to the list after a run only when you pass its
`region` to `PageScreen` as `listRef`, with a `listLabel`.

Give every bulk action an `icon`. On a medium wide screen the toolbar
shows only the icon. When one row was picked and it failed, show the
server's reason instead of a count. `failures[0].error` is `unknown`,
so cast it to `GraphFailure` and pass it through `graphError` and
`validationMessage`, as in [Failures in forms](#failures-in-forms).

### Dialogs

A write that fails never shows inside a dialog. Say why in a toast at
the bottom of the screen, as WordPress does, with `useToaster`.

A confirm on a list row is drawn inside the list's own modal. A
confirm that removes something, such as Revoke, hides the header, so
give that row action `hideModalHeader: true`. A plain confirm keeps
its header, the action's `modalHeader` or else its label. Either way,
give it `modalFocusOnMount: 'firstContentElement'`, so Cancel takes
the focus, and leave `modalSize` out, so it is 560px wide like every
WordPress row modal. Render `RunConfirm` with the question as its
child, the words of **Cancel** and of a button named after the
action, and `run`, which takes the picked rows and settles once your
list shows how it went. It keeps Cancel inert while the run goes and
closes once it settles:

```tsx
RenderModal: (props) => (
	<RunConfirm
		{...props}
		confirmLabel={__('Archive', 'acme-billing')}
		cancelLabel={__('Cancel', 'acme-billing')}
		run={(invoices) => bulk(invoices, archiveOne, archiveWords)}
	>
		{__('Archive the picked invoices?', 'acme-billing')}
	</RunConfirm>
),
```

For a row action that asks for more, the SDK passes on the bodies the
admin draws in a row modal. `ConfirmBody` asks a question you run
yourself: keep it `busy` while the write runs, then close the modal
and say how it went in a toast. `RenameBody` asks for a new name.
Give its button the label **Save**, as WordPress does, close the
modal the moment the reader saves, then write. Its Save is off only
while the name is blank, so a name left as it was can be sent.
`FormBody` holds any fields, with `kind` set to `edit`, `copy` or
`create` to space its buttons as WordPress does. A change to one row
closes when it is sent. A copy, or a change to several rows, keeps
the modal open with `busy` until the write ends.

When a write fails after its modal closed, give the reader back what
they typed. `useUnsavedText` holds what failed writes left unsaved,
by row id, for as long as the screen stays. Keep it in the hook that
builds your actions, never in the modal, and start the field from it.
`markLatest` marks a write as the latest one sent to a row, so a
failure keeps its text only when no later write went to that row:

```tsx
const save = (name: string) => {
	closeModal?.()
	renames.delete(invoice.id)
	const stillLatest = markLatest(renames, invoice.id)
	void graph.client
		.mutation(renameInvoiceMutation, { id: invoice.id, name })
		.toPromise()
		.then((result) => {
			if (result.error) {
				if (stillLatest()) {
					renames.set(invoice.id, name)
				}
				toaster.show(validationMessage(graphError(result.error), __('The name could not be updated.', 'acme-billing')))
				return
			}
			toaster.show(__('Name updated.', 'acme-billing'))
		})
}

<RenameBody
	name={renames.get(invoice.id) ?? invoice.name}
	fieldLabel={__('Name', 'acme-billing')}
	submitLabel={__('Save', 'acme-billing')}
	cancelLabel={__('Cancel', 'acme-billing')}
	onSubmit={save}
	onCancel={closeModal}
/>
```

An urql call answers with its `error` set rather than rejecting, so
check `result.error` instead of chaining `.catch`.

To confirm an action outside a list, such as a button in your
screen's header, use `ConfirmDialog` from
`@alphone/frontend-sdk/dialogs`. It draws the WordPress modal, a sheet
along the bottom of the screen on a phone, with Cancel focused. Leave
`title` out on a confirm that removes something, so the question is
the only text. What `onConfirm` returns decides when the dialog goes.
For a button, make it a plain function that returns nothing. The
dialog then closes at once while your write runs, and the button that
opened it shows the work. An `async` function always returns a
promise, which keeps the dialog open, so leave `onConfirm` plain.
Return the write's promise only for a confirm on a row of a list you
draw yourself, outside DataViews. The dialog then stays open with its
button busy until the write ends.

`FormDialog` from the same entry asks for a form from a button, as
WordPress opens Add Page: a title, the first field focused, and the
dialog left open after a failure, so the reader keeps what they typed.
The `/dialogs` entry loads the WordPress components only on the
screens that draw a dialog.

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
	},
],
```

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

A date column in a DataViews list is the one exception. Render it with
`listDate`, which writes the date and time the way WordPress lists do, in
the reader's language, such as Oct 13, 2026 5:04 pm:

```tsx
import { listDate } from '@alphone/frontend-sdk/dataviews'

render: ({ item }) => listDate(item.createdAt)
```

## Failures in forms

`validationMessage` shows a backend validation message verbatim and
falls back to your own copy for anything else. `ErrorNotice` is for a
failed load and for a form on its own page. A failure in a dialog or
in a run over list rows goes in a toast, as [Dialogs](#dialogs) says.

```tsx
<ErrorNotice>
	{validationMessage(create.error, 'The invoice could not be saved.')}
</ErrorNotice>
```

Throw `ValidationError` from your API layer when the backend rejects the
input, and anything else for a genuine failure.

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
| Put failure copy in a plain `Text` | `ErrorNotice` for a failed load or a form on its own page, a toast for a dialog or a list run | screen tests assert the alert role or the toast |
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
