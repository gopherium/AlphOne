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

## The contract

A screen puts its title top left and its actions top right, and the
title is the page's only first level heading. `PageScreen` gives you
that shape.

```tsx
import { Button, PageScreen } from '@alphone/frontend-sdk'
import { Link } from '@tanstack/react-router'

export function InvoicesScreen() {
	return (
		<PageScreen
			title="Invoices"
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

When one page has sections that each need their own address, give it
tabs. The Users page does this: the Users tab lists the accounts and
the API tokens tab lists the tokens. Both screens keep the title Users
and pass the same tabs, and each one marks its own tab as current:

```tsx
import { PageScreen, PageTab, PageTabs } from '@alphone/frontend-sdk'
import { Link } from '@tanstack/react-router'

<PageScreen
	title="Invoices"
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

Pass `aside` to show a second column beside the content, for details that
sit next to the main work. On a wide page it starts 320px wide. On a narrow
page it moves under the content, and an aside that renders nothing takes no
room.

A plugin's contact panel always renders in the contact page's aside, so
design it for a 320px column.

Put navigation and creation controls in `actions`. A form's submit
button belongs at the bottom of the form, not in the header.

Your screen renders in two shells without doing anything. On a wide
viewport it sits beside the navigation rail. Below 1024px the rail
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
	return (
		<EmptyState.Root className="godmin-empty">
			<EmptyState.Icon icon={people} />
			<EmptyState.Title>No invoices yet.</EmptyState.Title>
			<EmptyState.Description>Add one with New invoice.</EmptyState.Description>
		</EmptyState.Root>
	)
}
return <InvoiceTable rows={rows} />
```

`Text role="status"` announces loading politely. `ErrorNotice` announces
the failure as an alert, which plain copy in a `Text` never does.

`EmptyState.Root` always carries `className="godmin-empty"`. The design
system caps the empty state at a fixed width and leaves placing it to
the consumer, so without that class it sits flush left instead of
centered.

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
column readable.

For cursor paginated queries, `LoadMore` renders the next page button
and hides itself once every page is loaded.

```tsx
<LoadMore query={invoices}>Load more</LoadMore>
```

## Lists with DataViews

A short table you fill yourself fits the part above. A list a reader
searches, sorts, filters and pages through is a WordPress DataViews
list, like Contacts and Users. Import `DataViews` and its types from
`@alphone/frontend-sdk/dataviews`, and everything else from
`@alphone/frontend-sdk`.

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
		<PageScreen title={__('Invoices', 'acme-billing')} list>
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

### Bulk actions

An action with `supportsBulk: true` runs on every row the reader
ticked. `runEach` makes one call per row, all at once, and counts how
they went:

```tsx
import {
	__,
	_n,
	formatNumber,
	inbox,
	runEach,
	sprintf,
	useGraph,
	useToaster,
} from '@alphone/frontend-sdk'
import type { Action } from '@alphone/frontend-sdk/dataviews'

export function useArchiveAction(onFailure: (message: string | undefined) => void): Action<InvoiceRow> {
	const graph = useGraph()
	const toaster = useToaster()
	return {
		id: 'archive',
		label: __('Archive', 'acme-billing'),
		icon: inbox,
		supportsBulk: true,
		callback: async (invoices) => {
			onFailure(undefined)
			const { done, failures } = await runEach(invoices, (invoice) =>
				graph.client.mutation(archiveInvoiceMutation, { id: invoice.id }).toPromise(),
			)
			graph.refetch(['InvoicePage'])
			if (done > 0) {
				const template = _n('%s invoice archived.', '%s invoices archived.', done, 'acme-billing')
				toaster.show(sprintf(template, formatNumber(done)))
			}
			if (failures.length > 0) {
				const template = _n(
					'%s invoice could not be archived.',
					'%s invoices could not be archived.',
					failures.length,
					'acme-billing',
				)
				onFailure(sprintf(template, formatNumber(failures.length)))
			}
		},
	}
}
```

`runEach` answers `asked`, how many rows it got, `done`, how many calls
worked, and `failures`, each row that failed with its error, in the
order of the rows. A call fails when it throws, when its promise
rejects, or when it answers with its `error` set. An urql result with
an error counts as a failure, so the call hands back what `toPromise()`
gives it. `runEach` itself never fails, so one bad row never hides the
others.

The rest is the shape every AlphOne list follows. Clear the old notice
when the run starts. Refresh the list yourself, because DataViews never
tells the screen an action finished. Toast what worked, and show what
failed in an `ErrorNotice` above the list, never in a toast. The action
lives in a hook because `useGraph` and `useToaster` only work inside a
component, and the screen passes `onFailure` to show the message.

Give every bulk action an `icon`. On a medium wide screen the toolbar
shows only the icon. When one row was picked and it failed, show the
server's reason instead of a count. `failures[0].error` is `unknown`,
so cast it to `GraphFailure` and pass it through `graphError` and
`validationMessage`, as in [Failures in forms](#failures-in-forms).

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

### Testing a list screen

`renderPluginAt` from `@alphone/frontend-sdk/testing` mounts your
plugin at an address, under a router and a toaster, with a fake graph
client. Pass `session` to sign somebody in. The session goes straight
into the test's query cache, so no session request goes out. Without
one, `useSession` answers `null`, so a screen behind a capability shows
its refusal and `useAdminSettings` never asks for the page sizes.

```tsx
import { adminSession, graphql, HttpResponse, renderPluginAt, server } from '@alphone/frontend-sdk/testing'
import { screen } from '@testing-library/react'
import { expect, test } from 'vitest'

import { plugin } from '../index'

const managing = { ...adminSession, capabilities: ['manage_invoices'] }

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

## Failures in forms

`validationMessage` shows a backend validation message verbatim and
falls back to your own copy for anything else.

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

## Full bleed screens

Most screens sit on a padded canvas. A screen that fills its canvas edge
to edge, like a chat thread, opts out through its route.

```tsx
createRoute({
	getParentRoute: () => parent,
	path: 'threads/$threadId',
	component: ThreadScreen,
	staticData: { canvas: 'bleed' },
})
```

`bleed` removes the canvas padding and stops the canvas scrolling, so
the screen owns its own scrolling region. `padded` is the default and
never needs declaring.

A bleed screen builds its own chrome, so it uses `PageTitle` directly
instead of `PageScreen`. Every route needs exactly one first level
heading, including a bleed one.

```tsx
<header className="alphone-thread__header">
	<PageTitle variant="heading-md">{contactName}</PageTitle>
</header>
```

## Sidebar screens

A section that drills into its own sidebar declares a `Sidebar`
component on its route, and that component uses
`SidebarNavigationScreen`. It renders the back link, the section title,
and optional description, actions, and footer, and it moves focus to the
title on arrival.

## What not to do

Almost all of these fail the test suite rather than reaching a user. The
last one is a convention a reviewer will hold you to.

| Do not | Do instead | Caught by |
| ------ | ---------- | --------- |
| Write a raw `<h1>` or `<h2>` | `PageScreen title`, `PageTitle`, or a design system heading | source invariant |
| Leave a route without a page title | Give every route exactly one first level heading | rendered outline test |
| Add a route without listing it in the outline test | List it | rendered outline test |
| Use a bare `<EmptyState.Root>` | Add `className="godmin-empty"` | source invariant |
| Leave a table outside the scrolling region | Wrap it in `godmin-table-scroll` | source invariant |
| Let a screen spill sideways on a phone | Keep it inside the canvas | end to end fit sweep |
| Put failure copy in a plain `Text` | `ErrorNotice`, which announces it | screen tests assert the alert role |
| Cap the page width yourself | Let the page span the canvas | end to end geometry test |
| Hand roll a load more button | `LoadMore` | convention |

Three kinds of test carry these rules.

The **source invariant** reads every screen file and rejects the
patterns above, so a raw heading fails before anything renders.

The **rendered outline test** mounts every route and counts the first
level headings, so a screen that renders no title fails even though its
source looks clean. It also compares its route list against the router,
so adding a route without covering it fails too.

**Screen tests** find the error state by its alert role rather than by
its text, so replacing an announced failure with silent copy breaks
them.

The **fit sweep** seeds long names and an unbreakable word, then visits
every route on a phone sized viewport and names whatever spills past the
canvas. Content is allowed to be wider than the screen only when it
scrolls inside its own container.
