---
title: Add a translatable string
description: How to write a new interface string so it reaches translators, and what the gates check.
---

This page is for you if you are writing code that shows words to a
reader. Every such word has to reach the translators, and the repository
refuses to build if one does not.

## The short version

Wrap the string, name your text domain, then run `make pot` and commit
the template it regenerates. That is the whole contract. The rest of
this page explains what to wrap and the few places where wrapping needs
care.

## The functions

Everything a reader sees in AlphOne is React, so every string lives in
TypeScript. Import the gettext functions from `@alphone/frontend-sdk`,
never from anywhere else, so the whole application shares one
translation runtime.

```tsx
import { __, _x, sprintf } from '@alphone/frontend-sdk'

__('Add contact', 'alphone')
sprintf(__('A name runs to %(max)d characters at most.', 'alphone'), { max })
_x('Status', 'account status', 'alphone')
```

Use `__` for ordinary text. Use `sprintf` around it when a value goes
inside. A placeholder is named, like `%(max)d`, so a translator can move
it to wherever their language needs it. The letter says what fills the
hole, `d` for a number and `s` for text. Use `_x` when one English word
means two different things, which is covered below. The kit also exports
`_n` for wording that changes with a count, which no string needs yet.

## Name your own domain

The last argument is always the text domain, and it is always a literal.
Core code under `frontend/src` and `sdk/frontend` names `alphone`. A
plugin names its own domain, which is `alphone-` plus its folder name,
so the fields plugin writes `'alphone-fields'`. Each domain is its own
catalogue and its own translation project, so a string filed under the
wrong domain would ship in the wrong catalogue. A test reads every
source file and fails when a call names a domain the file does not own.

## A new plugin declares its catalogue

A plugin that shows words ships its own catalogues and tells the host
where they are. The manifest carries one `locale` entry.

```ts
const catalogs = import.meta.glob<{ default: Catalog }>('./languages/*.json')

export const plugin: FrontendPlugin = {
	id: 'whatsapp',
	locale: { domain: 'alphone-whatsapp', load: globCatalogs(catalogs) },
}
```

The host loads the catalogue matching the reader's language before the
first render, so a plugin never waits for its own words.

## Error messages have their own seam

The server answers a failed request with a short reason code. A template
turns that code into a sentence for the reader. A plugin declares its
templates as a function on the manifest, and the function runs when an
error arrives rather than when the file loads, so the catalogue the
reader loaded is the one that answers.

```ts
export function errorTemplates(): Record<string, string> {
	return {
		message_content_required: __('Write something to send.', 'alphone-whatsapp'),
		upstream_failed: __('WhatsApp did not accept the message.', 'alphone-whatsapp'),
	}
}
```

## Never wrap these

Class names, test ids, route paths, GraphQL field names, reason codes,
and any status or type value sent to the server. Those are identifiers,
not words, and translating one breaks the software.

Also leave alone the product names AlphOne and WhatsApp, and anything a
person typed into the product. A custom field's label reads as its
author wrote it, in whatever language they wrote it, and no catalogue
reaches it.

## When one word means two things

English reuses words that other languages separate. Status means one
thing for an account and another for a message. Give each use a context
and translators will see them as separate entries.

```tsx
_x('Status', 'account status', 'alphone')
_x('WhatsApp', 'admin section', 'alphone-whatsapp')
```

Only a word standing alone needs this. A word inside a whole sentence
carries its own meaning already.

## The trap that costs an afternoon

Calling a gettext function at the top level of a module runs it once,
when the file is first loaded. That can happen before the catalogue
arrives, so the text freezes in English and never changes again. Every
test still passes, because tests run in English.

The fix is to read the string when it is used rather than when the file
loads. The navigation items do this with a getter.

```ts
nav: [{
	get label() {
		return _x('WhatsApp', 'admin section', 'alphone-whatsapp')
	},
	to: '/whatsapp',
	icon: whatsappIcon,
}],
```

The getter costs nothing, so prefer it for anything imported eagerly.

## What the gates check

Run `make pot` after adding a string, and commit the templates it
regenerates. There is one template per domain.

A test rebuilds every template and compares it to the committed one byte
for byte, so a forgotten `make pot` fails the build. The domain test
above catches a string filed under a domain its file does not own. The
linter refuses bare text in the admin's markup, so a string that never
met a gettext function cannot ship. Further tests check every committed
translation, so an entry the template no longer names, a placeholder the
translation renamed, or a translation that fails to render all fail the
build. Nothing here depends on a human remembering.

If you also changed a translation, run `make catalogs` too. Compiled
catalogues are committed, and they are compared the same way.
