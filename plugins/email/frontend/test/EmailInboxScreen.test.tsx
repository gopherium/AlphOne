// SPDX-License-Identifier: AGPL-3.0-or-later

import { caution, inbox } from '@alphone/frontend-sdk'
import {
	HttpResponse,
	adminSession,
	graphql,
	paging,
	renderPluginAt,
	server,
	setViewport,
} from '@alphone/frontend-sdk/testing'
import { act, render, screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import type { ReactElement } from 'react'
import { beforeEach, expect, test } from 'vitest'

import { handlers, inboxPage, inboxThread, latestMail, mailbox, threads } from '../handlers'
import type { PageVariables } from '../handlers'
import { plugin } from '../index'

/** settled is how long a wait lasts for the rows. */
const settled = { timeout: 3000 }

/** widest are the column headings from 783px up, the date marked as the order the server answers in. */
const widest = ['From', 'Subject', 'Messages', 'Date↓']

/** asked records the variables of every page of threads the inbox asked for. */
let asked: Record<string, unknown>[] = []

/**
 * Serves the given threads from the inbox, paged as each request names, recording what each request asked.
 * @param held - The threads the inbox holds, the newest first.
 */
function serving(held: readonly Record<string, unknown>[]) {
	server.use(
		graphql.query('EmailInbox', ({ variables }) => {
			asked.push(variables)
			return HttpResponse.json(inboxPage(held, variables as PageVariables))
		}),
	)
}

/**
 * Serves the given answer for the workspace's mailbox.
 * @param body - The answer the mailbox read gets.
 */
function mailboxAnswers(body: Record<string, unknown>) {
	server.use(graphql.query('EmailMailbox', () => HttpResponse.json(body)))
}

/**
 * Holds the mailbox read until released, then serves the given answer.
 * @param body - The answer the mailbox read gets once released.
 * @returns Whether the read reached the server yet, beside what releases it.
 */
function mailboxHeld(body: Record<string, unknown>) {
	const held = { asked: false, release: () => {} }
	const answered = new Promise<void>((resolve) => {
		held.release = resolve
	})
	server.use(
		graphql.query('EmailMailbox', async () => {
			held.asked = true
			await answered
			return HttpResponse.json(body)
		}),
	)
	return held
}

/**
 * Builds the given number of threads, the newest first, each named by its place.
 * @param count - How many threads to build.
 * @returns The threads.
 */
function numbered(count: number) {
	return Array.from({ length: count }, (_, index) =>
		inboxThread(`019f6a00-0000-7000-8000-${String(index + 1).padStart(12, '0')}`, `Thread ${index + 1}`),
	)
}

/**
 * Renders the inbox for a signed-in admin.
 * @returns The router the plugin is mounted in.
 */
function openInbox() {
	return renderPluginAt(plugin, '/email', { session: adminSession }).router
}

/**
 * Returns the subjects the inbox rows show, in order.
 * @returns The subjects.
 */
async function shownSubjects(): Promise<(string | undefined)[]> {
	await screen.findByRole('table', {}, settled)
	await waitFor(() => expect(screen.getAllByRole('row').length).toBeGreaterThan(1), settled)
	return screen
		.getAllByRole('row')
		.slice(1)
		.map((row) => row.textContent ?? '')
		.map((text) => [...threads, ...numbered(9)].find((thread) => text.includes(thread.subject))?.subject)
}

/**
 * Returns the table row holding the given text.
 * @param text - A subject or a sender the row shows.
 * @returns The row queries.
 */
async function rowOf(text: string) {
	return within(await screen.findByRole('row', { name: new RegExp(text) }, settled))
}

/**
 * Returns the headings of the inbox columns, in order.
 * @returns The headings.
 */
async function columnHeadings(): Promise<string[]> {
	await rowOf('Delivery date')
	return screen.getAllByRole('columnheader').map((heading) => heading.textContent ?? '')
}

/**
 * Returns what the glyph of the empty state under the given title draws.
 * @param title - The title of the empty state.
 * @returns The path data of its drawing.
 */
async function emptyDrawing(title: string): Promise<string | null | undefined> {
	const heading = await screen.findByRole('heading', { level: 3, name: title }, settled)
	return heading.closest('.godmin-empty')?.querySelector('svg path')?.getAttribute('d')
}

/**
 * Returns the drawing a glyph shows.
 * @param glyph - The glyph.
 * @returns The path data of its drawing.
 */
function drawingOf(glyph: ReactElement): string | null | undefined {
	const { container, unmount } = render(glyph)
	const drawn = container.querySelector('path')?.getAttribute('d')
	unmount()
	return drawn
}

beforeEach(() => {
	asked = []
	server.use(...handlers)
	serving(threads)
})

test('lists the threads the newest first, in the order the server answers', async () => {
	openInbox()

	expect(await shownSubjects()).toEqual([
		'Delivery date',
		'Order confirmed',
		'Site visit on Friday',
		'Payment details',
		'Quote for the new lamp',
	])
})

test('says under the title whose mail the inbox reads', async () => {
	openInbox()

	const subtitle = await screen.findByText('Read the mail of info@example.com in one place.', {}, settled)
	expect(subtitle).toHaveClass('godmin-page__subtitle')
})

test('names the list region after the inbox', async () => {
	openInbox()

	expect(await screen.findByRole('region', { name: 'Inbox' }, settled)).toBeInTheDocument()
})

test('holds only the view options in the row above the list, since the server serves no search', async () => {
	openInbox()

	const options = await screen.findByRole('button', { name: 'View options' }, settled)
	expect(options.closest('.alphone-email-inbox__header')).not.toBeNull()
	expect(screen.queryByRole('searchbox')).not.toBeInTheDocument()
})

test('shows each sender by name, initials and address, the newest mail of the thread speaking', async () => {
	openInbox()

	const row = await rowOf('Delivery date')
	expect(row.getByText('Ada Lovelace')).toBeInTheDocument()
	expect(row.getByText('ada@example.com')).toBeInTheDocument()
	expect(row.getByText('A')).toHaveClass('godmin-avatar')
})

test('marks a signed thread holding files and previews its newest mail', async () => {
	openInbox()

	const row = await rowOf('Order confirmed')
	expect(row.getByText('Signed')).toBeInTheDocument()
	expect(row.getByRole('img', { name: 'Has attachments' })).toBeInTheDocument()
	expect(row.getByText('I confirm the order of two wicker lamps.')).toBeInTheDocument()
})

test('marks an encrypted thread and previews it with fixed words', async () => {
	openInbox()

	const row = await rowOf('Payment details')
	expect(row.getByText('Encrypted')).toBeInTheDocument()
	expect(row.getByText('Encrypted message')).toBeInTheDocument()
})

test('marks a thread the workspace wrote last with Sent beside its own address', async () => {
	serving([
		inboxThread('019f6a00-0000-7000-8000-000000000106', 'Re: Quote for the new lamp', {
			latest: latestMail('019f6a00-0000-7000-8000-000000000206', {
				direction: 'outbound',
				from: { __typename: 'EmailAddress', name: 'Info', email: 'info@example.com' },
				snippet: 'The quote is attached.',
			}),
		}),
	])
	openInbox()

	const row = await rowOf('Re: Quote for the new lamp')
	expect(row.getByText('Sent')).toBeInTheDocument()
	expect(row.getByText('info@example.com')).toBeInTheDocument()
})

test('previews a thread whose newest mail was read by its headers alone with fixed words', async () => {
	serving([
		inboxThread('019f6a00-0000-7000-8000-000000000107', 'Delivery date', {
			latest: latestMail('019f6a00-0000-7000-8000-000000000207', { bodyState: 'headers_only' }),
		}),
	])
	openInbox()

	expect((await rowOf('Delivery date')).getByText('No text could be read')).toBeInTheDocument()
})

test('counts the messages of each thread and dates it by its newest mail, the newest first', async () => {
	openInbox()

	const row = await rowOf('Quote for the new lamp')
	expect(row.getByText('3')).toBeInTheDocument()
	expect(screen.getAllByRole('columnheader').map((heading) => heading.textContent)).toEqual(widest)
	expect(screen.getByRole('columnheader', { name: /Date/ })).toHaveAttribute('aria-sort', 'descending')
})

test('opens nothing from a row yet', async () => {
	openInbox()

	const row = await rowOf('Delivery date')
	expect(row.queryByRole('link')).toBeNull()
	expect(row.queryByRole('button')).toBeNull()
})

test('pages the threads by the page size the admin settings name', async () => {
	paging([2, 4], 2)
	openInbox()

	expect(await shownSubjects()).toEqual(['Delivery date', 'Order confirmed'])
	await userEvent.click(screen.getByRole('button', { name: 'Next page' }))

	await waitFor(async () => expect(await shownSubjects()).toEqual(['Site visit on Friday', 'Payment details']))
	expect(asked.at(-1)).toEqual({ limit: 2, offset: 2 })
})

test('counts the pages from the total the server answers', async () => {
	paging([2, 4], 2)
	openInbox()

	await shownSubjects()
	expect(screen.getByText('of 3')).toBeInTheDocument()
})

test('offers as page sizes the ones the admin settings name', async () => {
	paging([2, 4], 2)
	openInbox()

	await shownSubjects()
	await userEvent.click(screen.getByRole('button', { name: 'View options' }))

	const sizes = within(await screen.findByRole('radiogroup', { name: 'Items per page' }))
	expect(sizes.getAllByRole('radio').map((size) => size.textContent)).toEqual(['2', '4'])
})

test('waits for the page size before it asks for threads', async () => {
	let release = () => {}
	const answered = new Promise<void>((resolve) => {
		release = resolve
	})
	server.use(
		graphql.query('AdminSettings', async () => {
			await answered
			return HttpResponse.json({
				data: {
					adminSettings: {
						__typename: 'AdminSettings',
						toastMilliseconds: 6000,
						toastActionMilliseconds: 10000,
						dialogWriteTimeoutMilliseconds: 60000,
						listPageSizes: [2, 4],
						listPageSize: 2,
						contactPageCap: 200,
						formatLocale: 'es-ES',
					},
				},
			})
		}),
	)
	openInbox()

	await screen.findByRole('button', { name: 'View options' }, settled)
	expect(asked).toEqual([])
	release()

	expect(await shownSubjects()).toEqual(['Delivery date', 'Order confirmed'])
	expect(asked).toEqual([{ limit: 2, offset: 0 }])
})

test('asks the size the graph serves when the page size could not be read', async () => {
	server.use(
		graphql.query('AdminSettings', () => HttpResponse.json({ data: null, errors: [{ message: 'internal error' }] })),
	)
	openInbox()

	expect(await shownSubjects()).toHaveLength(5)
	expect(asked).toEqual([{ limit: null, offset: 0 }])
})

test('reads the threads afresh each time the inbox opens', async () => {
	const router = openInbox()
	await shownSubjects()

	await act(() => router.navigate({ to: '/' }))
	await screen.findByText('Test host home')
	await act(() => router.navigate({ to: '/email' }))

	await waitFor(() => expect(asked).toHaveLength(2))
})

test('says no mailbox is connected, under the caution glyph and with no list, for a workspace with none', async () => {
	mailboxAnswers({ data: { emailMailbox: null } })
	openInbox()

	expect(await emptyDrawing('No mailbox is connected')).toBe(drawingOf(caution))
	expect(screen.getByText('Mail shows here once a mailbox is connected to this workspace.')).toBeInTheDocument()
	expect(screen.queryByRole('table')).not.toBeInTheDocument()
	expect(screen.queryByText(/Read the mail of/)).not.toBeInTheDocument()
})

test('centres the state with no mailbox in a callout box of its own inside the list region', async () => {
	mailboxAnswers({ data: { emailMailbox: null } })
	openInbox()

	const heading = await screen.findByRole('heading', { level: 3, name: 'No mailbox is connected' }, settled)
	const callout = heading.closest('.alphone-email-inbox__callout')
	expect(callout).not.toBeNull()
	expect(callout).toHaveStyle({ alignItems: 'center', justifyContent: 'center' })
	expect(screen.getByRole('region', { name: 'Inbox' })).toContainElement(callout as HTMLElement)
})

test('asks for no thread before it knows the workspace holds a mailbox', async () => {
	const held = mailboxHeld({ data: { emailMailbox: null } })
	paging([2, 4], 2)
	openInbox()

	await screen.findByRole('heading', { level: 2, name: 'Email' }, settled)
	await waitFor(() => expect(held.asked).toBe(true), settled)
	expect(asked).toEqual([])
	held.release()

	expect(await emptyDrawing('No mailbox is connected')).toBe(drawingOf(caution))
	expect(asked).toEqual([])
})

test('shows a spinner alone under the title until the mailbox read answers, then its subtitle and list', async () => {
	const held = mailboxHeld({ data: { emailMailbox: mailbox } })
	openInbox()

	await screen.findByRole('heading', { level: 2, name: 'Email' }, settled)
	await waitFor(() => expect(held.asked).toBe(true), settled)
	expect(screen.getByRole('status')).toHaveTextContent('Loading mail…')
	expect(screen.queryByRole('button', { name: 'View options' })).toBeNull()
	expect(screen.queryByRole('table')).toBeNull()
	held.release()

	expect(await screen.findByRole('button', { name: 'View options' }, settled)).toBeInTheDocument()
	expect(screen.getByText('Read the mail of info@example.com in one place.')).toBeInTheDocument()
	expect(await shownSubjects()).toHaveLength(5)
	expect(screen.queryByText('Loading mail…')).toBeNull()
})

test('says the inbox is set up and waits for mail, under the inbox glyph, when the mailbox holds none', async () => {
	serving([])
	openInbox()

	expect(await emptyDrawing("You're set up. No mail yet.")).toBe(drawingOf(inbox))
	expect(screen.getByText('New mail will appear here.')).toBeInTheDocument()
	expect(screen.getByText('Read the mail of info@example.com in one place.')).toBeInTheDocument()
})

test('reports when the threads cannot be loaded', async () => {
	server.use(graphql.query('EmailInbox', () => HttpResponse.json({ data: null, errors: [{ message: 'nope' }] })))
	openInbox()

	expect(await screen.findByRole('alert', {}, settled)).toHaveTextContent('Mail could not be loaded.')
})

test('reports when the mailbox cannot be read', async () => {
	mailboxAnswers({ data: null, errors: [{ message: 'nope' }] })
	openInbox()

	expect(await screen.findByRole('alert', {}, settled)).toHaveTextContent('Mail could not be loaded.')
	expect(screen.queryByRole('table')).not.toBeInTheDocument()
})

test('keeps the sender column alone up to 782px, the subject and its marks under the sender', async () => {
	setViewport({ width: 782 })
	openInbox()

	expect(await columnHeadings()).toEqual(['From'])
	const row = await rowOf('Order confirmed')
	expect(row.getByText('Maria Perez')).toBeInTheDocument()
	expect(row.getByText('Signed')).toBeInTheDocument()
	expect(row.queryByText('maria.perez@example.com')).toBeNull()
	expect(row.queryByText('I confirm the order of two wicker lamps.')).toBeNull()
})

test('shows every column from 783px up', async () => {
	setViewport({ width: 783 })
	openInbox()

	expect(await columnHeadings()).toEqual(widest)
})

test('gives the subject column the room the others leave on a wide screen, beside the density picked', async () => {
	setViewport({ width: 1280 })
	const router = openInbox()
	await rowOf('Delivery date')

	expect(screen.getByRole('columnheader', { name: /Subject/ })).toHaveStyle({ width: '100%', maxWidth: '0px' })
	await userEvent.click(screen.getByRole('button', { name: 'View options' }))
	await userEvent.click(await screen.findByRole('radio', { name: 'Compact' }, settled))

	await waitFor(() => expect(screen.getByRole('table')).toHaveClass('has-compact-density'))
	expect(screen.getByRole('columnheader', { name: /Subject/ })).toHaveStyle({ width: '100%', maxWidth: '0px' })
	expect(router.state.location.search).toEqual({})
})

test('offers on a narrow screen no property its sender cell leaves out, and names each one offered once', async () => {
	setViewport({ width: 390 })
	openInbox()

	await rowOf('Delivery date')
	await userEvent.click(screen.getByRole('button', { name: 'View options' }))

	await screen.findByRole('heading', { level: 2, name: 'Appearance' }, settled)
	const labels = document.querySelectorAll('.dataviews-view-config__properties .dataviews-view-config__label')
	const offered = [...labels].map((label) => label.textContent ?? '')
	expect(offered.filter((label) => !['From', 'Avatar', 'Subject'].includes(label))).toEqual([])
	expect(new Set(offered).size).toBe(offered.length)
})

test('keeps the order the server answers in whatever sort the address names', async () => {
	renderPluginAt(plugin, '/email?sort=subject&order=asc', { session: adminSession })

	expect(await columnHeadings()).toEqual(widest)
	expect(screen.getByRole('columnheader', { name: /Subject/ })).not.toHaveAttribute('aria-sort')
})

test('lays the inbox out as a table on a phone too, the sender over the subject', async () => {
	setViewport({ width: 390 })
	openInbox()

	expect(await columnHeadings()).toEqual(['From'])
	expect((await rowOf('Payment details')).getByText('Encrypted')).toBeInTheDocument()
})

test('brings the columns back once the screen widens, after a page turned on a narrow screen', async () => {
	paging([2, 4], 2)
	setViewport({ width: 700 })
	openInbox()
	await shownSubjects()

	await userEvent.click(screen.getByRole('button', { name: 'Next page' }))
	await waitFor(async () => expect(await shownSubjects()).toEqual(['Site visit on Friday', 'Payment details']))
	act(() => setViewport({ width: 1280 }))

	await waitFor(() =>
		expect(screen.getAllByRole('columnheader').map((heading) => heading.textContent)).toEqual(widest),
	)
	expect((await rowOf('Site visit on Friday')).getByText('maria.perez@example.com')).toBeInTheDocument()
})
