// SPDX-License-Identifier: AGPL-3.0-or-later

import { comment } from '@alphone/frontend-sdk'
import { HttpResponse, adminSession, graphql, renderPluginAt, server, setViewport } from '@alphone/frontend-sdk/testing'
import { cleanup, render, screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { beforeEach, expect, test } from 'vitest'

import { handlers, twoNumbers } from '../handlers'
import { plugin } from '../index'

beforeEach(() => {
	sessionStorage.clear()
	server.use(...handlers)
})

/**
 * Returns the sidebar the host draws.
 * @returns The region named Navigation.
 */
function sidebar(): Promise<HTMLElement> {
	return screen.findByRole('region', { name: 'Navigation' })
}

test('shows a spinner in place of the sidebar rows while the conversations arrive', async () => {
	server.use(graphql.query('WhatsAppConversations', () => new Promise(() => {})))
	renderPluginAt(plugin, '/whatsapp', { session: adminSession })

	const status = await screen.findByRole('status')
	expect(status).toHaveTextContent('Loading conversations…')
	expect(status.closest('.godmin-loading-screen')).not.toBeNull()
})

test('lists conversations from the graph, most recent first', async () => {
	renderPluginAt(plugin, '/whatsapp', { session: adminSession })

	expect(await screen.findByText('John Doe')).toBeInTheDocument()
	expect(screen.getByText('María Pérez')).toBeInTheDocument()

	const names = screen.getAllByRole('listitem').map((item) => item.textContent)
	expect(names[0]).toContain('John Doe')
	expect(names[1]).toContain('María Pérez')
})

test('draws each conversation as a row of the sub menu list, as every sidebar row', async () => {
	renderPluginAt(plugin, '/whatsapp', { session: adminSession })

	const shown = await sidebar()
	const row = await within(shown).findByRole('link', { name: /John Doe/ })
	expect(row).toHaveClass('godmin-navigation-item')
	expect(within(shown).getByRole('list')).toContainElement(row)
	expect(row.querySelector('.alphone-conversation .alphone-conversation__name')).toHaveTextContent('John Doe')
})

test('draws the comment glyph on every conversation row, so the names line up under the sub menu title', async () => {
	const { container, unmount } = render(comment)
	const drawing = container.querySelector('svg')?.innerHTML
	unmount()
	renderPluginAt(plugin, '/whatsapp', { session: adminSession })

	const shown = await sidebar()
	await within(shown).findByRole('link', { name: /John Doe/ })
	const rows = within(within(shown).getByRole('list')).getAllByRole('link')
	expect(rows.map((row) => row.querySelector('svg')?.innerHTML)).toEqual(rows.map(() => drawing))
})

test('marks the open conversation as the current row', async () => {
	renderPluginAt(plugin, '/whatsapp/conversations/019f4a00-0000-7000-8000-000000000001', { session: adminSession })

	const row = await within(await sidebar()).findByRole('link', { name: /John Doe/ })
	expect(row).toHaveAttribute('aria-current', 'page')
	expect(within(await sidebar()).getByRole('link', { name: /María Pérez/ })).not.toHaveAttribute('aria-current')
})

test('shows each conversation preview and last activity', async () => {
	renderPluginAt(plugin, '/whatsapp', { session: adminSession })

	expect(
		await screen.findByText('I can pick it up after 5pm.'),
	).toBeInTheDocument()
	expect(screen.getByText('hello')).toBeInTheDocument()
	expect(screen.getAllByText('06/07/2026')).toHaveLength(2)
	expect(screen.getByText('05/07/2026')).toBeInTheDocument()
	expect(screen.getAllByText('status')).toHaveLength(3)

	const quietRow = screen.getByText('Quiet Contact').closest('li')
	expect(
		quietRow?.querySelector('.alphone-conversation__preview')?.textContent,
	).toBe('')
})

test('shows an empty state when no conversations exist', async () => {
	server.use(
		graphql.query('WhatsAppConversations', () =>
			HttpResponse.json({ data: { whatsAppConversations: [] } }),
		),
	)

	renderPluginAt(plugin, '/whatsapp', { session: adminSession })

	const empty = await screen.findByText(/no conversations yet/i)
	expect(empty).toHaveAttribute('role', 'status')
	expect(within(await sidebar()).queryByRole('list')).toBeNull()
})

test('reports when conversations cannot be loaded', async () => {
	server.use(
		graphql.query('WhatsAppConversations', () =>
			HttpResponse.json({ data: null, errors: [{ message: 'internal error' }] }),
		),
	)

	renderPluginAt(plugin, '/whatsapp', { session: adminSession })

	expect(await screen.findByRole('alert')).toHaveTextContent(
		/could not be loaded/i,
	)
})

test('shows no number and no filter while the workspace holds one number', async () => {
	renderPluginAt(plugin, '/whatsapp', { session: adminSession })

	await screen.findByText('John Doe')

	expect(screen.queryByText('Sales line')).toBeNull()
	expect(screen.queryByRole('combobox')).toBeNull()
})

test('names the number a conversation came in on once the workspace holds more than one', async () => {
	server.use(twoNumbers)
	renderPluginAt(plugin, '/whatsapp', { session: adminSession })

	const shown = await sidebar()
	const number = await within(shown).findByText('Sales line')
	expect(number).toHaveClass('alphone-conversation__number')
	expect(within(shown).getByRole('link', { name: /John Doe/ })).toContainElement(number)
	expect(within(shown).getByRole('link', { name: /María Pérez/ })).toHaveTextContent('5550002')
})

test('names the number to a screen reader as it names the status', async () => {
	server.use(twoNumbers)
	renderPluginAt(plugin, '/whatsapp', { session: adminSession })

	await screen.findByText('Sales line')

	expect(screen.getAllByText('WhatsApp number')).toHaveLength(2)
})

test('leaves the number out of a row that came in on none', async () => {
	server.use(twoNumbers)
	renderPluginAt(plugin, '/whatsapp', { session: adminSession })

	await screen.findByText('Sales line')

	const row = screen.getByRole('link', { name: /Quiet Contact/ })
	expect(row.querySelector('.alphone-conversation__number')).toBeNull()
})

test('offers every number in the filter by its label, or else its identifier', async () => {
	server.use(twoNumbers)
	renderPluginAt(plugin, '/whatsapp', { session: adminSession })

	await userEvent.click(await screen.findByRole('combobox', { name: 'Filter by number' }))

	const offered = await screen.findAllByRole('option')
	expect(offered.map((option) => option.textContent)).toEqual(['All numbers', 'Sales line', '5550002'])
})

test('starts the filter on every number', async () => {
	server.use(twoNumbers)
	renderPluginAt(plugin, '/whatsapp', { session: adminSession })

	const filter = await screen.findByRole('combobox', { name: 'Filter by number' })
	expect(filter).toHaveTextContent('All numbers')
	expect(filter).not.toHaveAttribute('data-placeholder')
})

/**
 * Chooses one option of the number filter.
 * @param name - The option to choose.
 */
async function chooseNumber(name: string) {
	await userEvent.click(await screen.findByRole('combobox', { name: 'Filter by number' }))
	await userEvent.click(await screen.findByRole('option', { name }))
}

/**
 * Waits until the number filter reads one option.
 * @param name - The option it should read.
 */
async function filterReads(name: string) {
	const filter = await screen.findByRole('combobox', { name: 'Filter by number' })
	await waitFor(() => expect(filter).toHaveTextContent(name))
}

test('reads the conversations of the number the filter picks, then every one again', async () => {
	const asked: unknown[] = []
	server.use(
		twoNumbers,
		graphql.query('WhatsAppConversations', ({ variables }) => {
			asked.push(variables.number)
			return HttpResponse.json({ data: { whatsAppConversations: [] } })
		}),
	)
	renderPluginAt(plugin, '/whatsapp', { session: adminSession })
	await screen.findByRole('combobox', { name: 'Filter by number' })

	await chooseNumber('5550002')
	await waitFor(() => expect(asked).toContain('5550002'))
	expect(await screen.findByRole('combobox', { name: 'Filter by number' })).toHaveTextContent('5550002')

	await chooseNumber('All numbers')
	await waitFor(() => expect(asked.at(-1)).toBeUndefined())
})

/**
 * Opens the phone drawer holding the sidebar.
 * @returns The drawer.
 */
async function openDrawer(): Promise<HTMLElement> {
	await userEvent.click(await screen.findByRole('button', { name: 'Open navigation panel' }))
	return screen.findByRole('dialog', { name: 'Navigation' })
}

test('keeps the phone drawer open while the filter narrows the list', async () => {
	setViewport({ matches: true })
	server.use(twoNumbers)
	const { router } = renderPluginAt(plugin, '/whatsapp', { session: adminSession })
	const drawer = await openDrawer()

	await chooseNumber('5550002')

	await filterReads('5550002')
	expect(screen.getByRole('dialog', { name: 'Navigation' })).toBe(drawer)
	expect(router.state.location.search).toEqual({})
})

test('keeps the chosen number when the phone drawer closes on a conversation and reopens', async () => {
	setViewport({ matches: true })
	server.use(twoNumbers)
	renderPluginAt(plugin, '/whatsapp', { session: adminSession })
	const drawer = await openDrawer()
	await chooseNumber('Sales line')

	await userEvent.click(await within(drawer).findByRole('link', { name: /John Doe/ }))
	await waitFor(() => expect(screen.queryByRole('dialog')).toBeNull())

	const reopened = await openDrawer()
	expect(await within(reopened).findByRole('combobox', { name: 'Filter by number' })).toHaveTextContent('Sales line')
})

test('forgets the chosen number once the filter goes back to every number', async () => {
	server.use(twoNumbers)
	renderPluginAt(plugin, '/whatsapp', { session: adminSession })
	await chooseNumber('Sales line')
	await chooseNumber('All numbers')
	await filterReads('All numbers')

	cleanup()
	renderPluginAt(plugin, '/whatsapp', { session: adminSession })

	expect(await screen.findByRole('combobox', { name: 'Filter by number' })).toHaveTextContent('All numbers')
})

test('reads every conversation once the workspace no longer holds the chosen number', async () => {
	server.use(twoNumbers)
	renderPluginAt(plugin, '/whatsapp', { session: adminSession })
	await chooseNumber('5550002')
	await filterReads('5550002')
	cleanup()
	const asked: unknown[] = []
	server.use(
		graphql.query('WhatsAppNumbers', () =>
			HttpResponse.json({
				data: {
					whatsAppNumbers: [
						{ __typename: 'WhatsAppNumber', identifier: '5550001', label: 'Sales line' },
						{ __typename: 'WhatsAppNumber', identifier: '5550003', label: '' },
					],
				},
			}),
		),
		graphql.query('WhatsAppConversations', ({ variables }) => {
			asked.push(variables.number)
			return HttpResponse.json({ data: { whatsAppConversations: [] } })
		}),
	)

	renderPluginAt(plugin, '/whatsapp', { session: adminSession })

	expect(await screen.findByRole('combobox', { name: 'Filter by number' })).toHaveTextContent('All numbers')
	expect(await screen.findByText('No conversations yet.')).toBeInTheDocument()
	expect(asked).not.toContain('5550002')
})

test('reads every conversation once a workspace is down to one number with no filter to show', async () => {
	server.use(twoNumbers)
	renderPluginAt(plugin, '/whatsapp', { session: adminSession })
	await chooseNumber('Sales line')
	await filterReads('Sales line')
	cleanup()
	let numbersRead = false
	const asked: unknown[] = []
	server.use(
		graphql.query('WhatsAppNumbers', () => {
			numbersRead = true
			return HttpResponse.json({
				data: { whatsAppNumbers: [{ __typename: 'WhatsAppNumber', identifier: '5550001', label: 'Sales line' }] },
			})
		}),
		graphql.query('WhatsAppConversations', ({ variables }) => {
			asked.push(variables.number)
			return HttpResponse.json({ data: { whatsAppConversations: [] } })
		}),
	)

	renderPluginAt(plugin, '/whatsapp', { session: adminSession })

	await waitFor(() => expect(numbersRead).toBe(true))
	expect(await screen.findByText('No conversations yet.')).toBeInTheDocument()
	expect(asked).not.toContain('5550001')
})

test('keeps the filter on screen and says so when a number has no conversations', async () => {
	server.use(
		twoNumbers,
		graphql.query('WhatsAppConversations', ({ variables }) =>
			HttpResponse.json({
				data: {
					whatsAppConversations: variables.number === '5550002' ? [] : [
						{
							__typename: 'WhatsAppConversation',
							id: '019f4a00-0000-7000-8000-000000000001',
							status: 'open',
							lastActivityAt: '2026-07-06T10:05:00Z',
							lastMessagePreview: 'hello',
							number: 'Sales line',
							contact: {
								__typename: 'Contact',
								id: '019f4a00-0000-7000-8000-0000000000a1',
								name: 'John Doe',
							},
						},
					],
				},
			}),
		),
	)
	renderPluginAt(plugin, '/whatsapp', { session: adminSession })
	await screen.findByText('John Doe')

	await chooseNumber('5550002')

	expect(await screen.findByText('No conversations on this number.')).toHaveAttribute('role', 'status')
	expect(screen.getByRole('combobox', { name: 'Filter by number' })).toBeInTheDocument()
})

test('keeps the filter on screen while the conversations cannot be loaded', async () => {
	server.use(
		twoNumbers,
		graphql.query('WhatsAppConversations', () =>
			HttpResponse.json({ data: null, errors: [{ message: 'internal error' }] }),
		),
	)
	renderPluginAt(plugin, '/whatsapp', { session: adminSession })

	expect(await screen.findByRole('alert')).toHaveTextContent(/could not be loaded/i)
	expect(await screen.findByRole('combobox', { name: 'Filter by number' })).toBeInTheDocument()
})
