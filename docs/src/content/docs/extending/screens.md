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
				<Button variant="solid" render={<Link to="/invoices/new" />}>
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

Put navigation and creation controls in `actions`. A form's submit
button belongs at the bottom of the form, not in the header.

Your screen renders in two shells without doing anything. On a wide
viewport it sits beside the navigation rail. Below 1024px the rail
becomes a drawer behind a menu button, and below 640px the canvas meets
the screen edges and pads tighter. Build one screen and check it at both
sizes.

## Who is signed in

A screen reads the signed-in account with `useSession`. It answers the
account's id, email, name, and role, or `null` when nobody is signed
in.

```tsx
import { useSession } from '@alphone/frontend-sdk'

export function InvoicesScreen() {
	const session = useSession()
	const manages = session?.role === 'admin'
	…
}
```

A role is either `admin` or `member`. Anything else counts as
`member`, so a screen that gets an answer it does not recognise hides
the control rather than offering it.

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
