// SPDX-License-Identifier: AGPL-3.0-or-later

import { file } from '@alphone/frontend-sdk'
import { listDate } from '@alphone/frontend-sdk/dataviews'
import type { View } from '@alphone/frontend-sdk/dataviews'
import { badgeClasses, textClasses } from '@alphone/frontend-sdk/testing'
import { render, screen, within } from '@testing-library/react'
import type { ComponentType } from 'react'
import { expect, test } from 'vitest'

import { inboxThread, latestMail } from '../handlers'
import { drawnView, inboxFields, newestFirst, withoutDrawnStyles } from '../inboxFields'
import type { InboxThread } from '../inboxFields'

/**
 * Builds one thread as the inbox lists it.
 * @param fields - The fields overriding a thread of one whole mail from Maria Perez.
 * @param mail - The fields overriding its newest mail.
 * @returns The thread.
 */
function thread(fields: Record<string, unknown> = {}, mail: Record<string, unknown> = {}): InboxThread {
	return inboxThread('019f6a00-0000-7000-8000-000000000301', 'Site visit on Friday', {
		latest: latestMail('019f6a00-0000-7000-8000-000000000401', {
			snippet: 'Could you come by on Friday morning?',
			...mail,
		}),
		...fields,
	}) as unknown as InboxThread
}

/**
 * Renders what one field shows for a thread.
 * @param id - The field to render.
 * @param item - The thread it renders.
 * @returns The element holding what the field drew.
 */
function shown(id: string, item: InboxThread): HTMLElement {
	const field = inboxFields().find((each) => each.id === id)
	const Render = field?.render as ComponentType<{ item: InboxThread; field: unknown }>
	return render(<Render item={item} field={field} />).container
}

/**
 * Returns the markup a glyph draws inside its svg.
 * @param held - The element holding the glyph.
 * @returns The markup.
 */
function drawingIn(held: Element): string | undefined {
	return held.querySelector('svg')?.innerHTML
}

test('lays the fields out with the sender as the title, the subject, the count and the date', () => {
	expect(inboxFields().map((field) => field.id)).toEqual([
		'avatar',
		'from',
		'address',
		'subjectLine',
		'subject',
		'messages',
		'date',
	])
})

test('offers no sort and no filter, since the server serves neither', () => {
	for (const field of inboxFields()) {
		expect({ id: field.id, sorts: field.enableSorting, filters: field.filterBy }).toEqual({
			id: field.id,
			sorts: false,
			filters: false,
		})
	}
})

test('keeps the sender and the two lines that sit under it out of the columns a reader hides', () => {
	const locked = inboxFields()
		.filter((field) => field.enableHiding === false)
		.map((field) => field.id)

	expect(locked).toEqual(['from', 'address', 'subjectLine'])
})

test('names the line under the sender Email, the word a list of people uses for an address of mail', () => {
	expect(inboxFields().find((field) => field.id === 'address')?.label).toBe('Email')
})

test('draws the initials of the newest sender', () => {
	const drawn = shown('avatar', thread())

	expect(drawn.querySelector('.godmin-avatar')).toHaveTextContent('M')
})

test('draws no letter for a mail that names no sender', () => {
	const drawn = shown('avatar', thread({}, { from: null }))

	expect(drawn.querySelector('.godmin-avatar')?.textContent).toBe('')
})

test('names the newest sender with no badge when the mail came in', () => {
	const drawn = shown('from', thread())

	expect(drawn).toHaveTextContent('Maria Perez')
	expect(within(drawn).queryByText('Sent')).toBeNull()
})

test('marks a thread the workspace wrote last with a quiet Sent badge beside the sender', () => {
	const drawn = shown(
		'from',
		thread({}, {
			direction: 'outbound',
			from: { __typename: 'EmailAddress', name: 'Info', email: 'info@example.com' },
		}),
	)

	expect(drawn).toHaveTextContent('Info')
	expect([...within(drawn).getByText('Sent').classList]).toEqual(badgeClasses('none'))
})

test('shows the address of the newest sender in small body text and weak foreground, capped as the name is', () => {
	const address = within(shown('address', thread())).getByText('maria.perez@example.com')

	expect([...address.classList]).toEqual(expect.arrayContaining(textClasses('body-sm')))
	expect(address).toHaveClass('alphone-email-inbox__weak', 'alphone-email-inbox__address')
})

test('shows no address under a sender whose name is the address, which the name line already shows', () => {
	const namedByAddress = {
		__typename: 'EmailAddress',
		name: 'maria.perez@example.com',
		email: 'maria.perez@example.com',
	}

	expect(shown('address', thread({}, { from: namedByAddress }))).toBeEmptyDOMElement()
})

test('shows no address under a sender who gave only the address, which the name line already shows', () => {
	const drawn = shown(
		'address',
		thread({}, { from: { __typename: 'EmailAddress', name: '', email: 'maria.perez@example.com' } }),
	)

	expect(drawn).toBeEmptyDOMElement()
})

test('shows no address when the newest mail names no sender', () => {
	expect(shown('address', thread({}, { from: null }))).toBeEmptyDOMElement()
})

test('shows the subject over a preview of the newest mail in small body text and the weak foreground', () => {
	const drawn = shown('subject', thread())

	expect(within(drawn).getByText('Site visit on Friday')).toBeInTheDocument()
	const preview = within(drawn).getByText('Could you come by on Friday morning?')
	expect([...preview.classList]).toEqual(expect.arrayContaining(textClasses('body-sm')))
	expect(preview).toHaveClass('alphone-email-inbox__text', 'alphone-email-inbox__weak')
})

test('marks an encrypted mail with a quiet badge beside the subject and previews it with fixed words', () => {
	const drawn = shown('subject', thread({}, { encrypted: true, snippet: '' }))

	expect([...within(drawn).getByText('Encrypted').classList]).toEqual(badgeClasses('none'))
	expect(within(drawn).getByText('Encrypted message')).toBeInTheDocument()
})

test('marks a signed mail with a quiet badge beside the subject and keeps its preview', () => {
	const drawn = shown('subject', thread({}, { signed: true }))

	expect([...within(drawn).getByText('Signed').classList]).toEqual(badgeClasses('none'))
	expect(within(drawn).getByText('Could you come by on Friday morning?')).toBeInTheDocument()
})

test('previews a mail read by its headers alone with fixed words', () => {
	const drawn = shown('subject', thread({}, { bodyState: 'headers_only', snippet: '' }))

	expect(within(drawn).getByText('No text could be read')).toBeInTheDocument()
})

test('draws the file glyph, named for a screen reader, beside the subject of a thread holding files', () => {
	shown('subject', thread({ hasAttachments: true }))

	const mark = screen.getByRole('img', { name: 'Has attachments' })
	expect(drawingIn(mark)).toBe(drawingIn(render(file).container))
})

test('names the file glyph by its title alone, read once and shown on hover, in a box centred on the line', () => {
	shown('subject', thread({ hasAttachments: true }))

	const mark = screen.getByRole('img', { name: 'Has attachments' })
	expect(mark).toHaveAttribute('title', 'Has attachments')
	expect(mark).not.toHaveAttribute('aria-label')
	expect(mark).toHaveClass('alphone-email-inbox__file')
})

test('holds the badges and the file glyph beside a subject in one marks box, and none when there is no mark', () => {
	const marked = shown('subject', thread({ hasAttachments: true }, { encrypted: true, signed: true, snippet: '' }))
	const boxes = marked.querySelectorAll('.alphone-email-inbox__marks')

	expect(boxes).toHaveLength(1)
	expect(boxes[0].parentElement).toHaveClass('alphone-email-inbox__line')
	const box = within(boxes[0] as HTMLElement)
	expect(box.getByText('Encrypted')).toBeInTheDocument()
	expect(box.getByText('Signed')).toBeInTheDocument()
	expect(box.getByRole('img', { name: 'Has attachments' })).toBeInTheDocument()
	expect(shown('subject', thread()).querySelector('.alphone-email-inbox__marks')).toBeNull()
})

test('draws no file glyph for a thread holding no file', () => {
	shown('subject', thread())

	expect(screen.queryByRole('img', { name: 'Has attachments' })).toBeNull()
})

test('shows the subject line with its marks and no preview under the sender on a narrow screen', () => {
	const drawn = shown('subjectLine', thread({ hasAttachments: true }, { signed: true }))

	expect(within(drawn).getByText('Site visit on Friday')).toBeInTheDocument()
	expect(within(drawn).getByText('Signed')).toBeInTheDocument()
	expect(within(drawn).getByRole('img', { name: 'Has attachments' })).toBeInTheDocument()
	expect(within(drawn).queryByText('Could you come by on Friday morning?')).toBeNull()
})

test('puts the whole text of each line that can cut short in its title, on a wide and a narrow screen', () => {
	const item = thread()
	const cell = within(shown('subject', item))

	expect(within(shown('from', item)).getByText('Maria Perez')).toHaveAttribute('title', 'Maria Perez')
	const address = within(shown('address', item)).getByText('maria.perez@example.com')
	expect(address).toHaveAttribute('title', 'maria.perez@example.com')
	expect(cell.getByText('Site visit on Friday')).toHaveAttribute('title', 'Site visit on Friday')
	const preview = cell.getByText('Could you come by on Friday morning?')
	expect(preview).toHaveAttribute('title', 'Could you come by on Friday morning?')
	expect(within(shown('subjectLine', item)).getByText('Site visit on Friday')).toHaveAttribute(
		'title',
		'Site visit on Friday',
	)
})

test('puts the fixed words of a sender, a subject or a preview it cannot show in their titles too', () => {
	const item = thread({ subject: '' }, { from: null, encrypted: true, snippet: '' })

	expect(within(shown('from', item)).getByText('(no sender)')).toHaveAttribute('title', '(no sender)')
	expect(within(shown('subject', item)).getByText('(no subject)')).toHaveAttribute('title', '(no subject)')
	expect(within(shown('subject', item)).getByText('Encrypted message')).toHaveAttribute('title', 'Encrypted message')
})

test('counts the messages of a thread in the format language', () => {
	expect(shown('messages', thread({ messageCount: 12345 }))).toHaveTextContent('12.345')
})

test('dates a thread by its newest mail, as every list dates its rows', () => {
	expect(shown('date', thread({ lastMessageAt: '2026-11-19T12:00:00Z' }))).toHaveTextContent(
		listDate('2026-11-19T12:00:00Z'),
	)
})

test('takes the column style a wide screen draws back out of a changed view, keeping the density picked', () => {
	const held = { type: 'table', fields: ['subject'], layout: { density: 'compact' } } as View
	const drawn = drawnView(held, false)

	expect(drawn.layout).toEqual({ density: 'compact', styles: { subject: { width: '100%', maxWidth: 0 } } })
	expect(withoutDrawnStyles(drawn)).toEqual({ ...held, sort: newestFirst })
})
