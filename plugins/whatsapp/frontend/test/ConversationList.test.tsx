// SPDX-License-Identifier: AGPL-3.0-or-later

import { comment } from '@alphone/frontend-sdk'
import { HttpResponse, adminSession, graphql, renderPluginAt, server } from '@alphone/frontend-sdk/testing'
import { render, screen, within } from '@testing-library/react'
import { beforeEach, expect, test } from 'vitest'

import { handlers } from '../handlers'
import { plugin } from '../index'

beforeEach(() => server.use(...handlers))

/**
 * Returns the sidebar the host draws.
 * @returns The region named Navigation.
 */
function sidebar(): Promise<HTMLElement> {
	return screen.findByRole('region', { name: 'Navigation' })
}

test('ghosts the sidebar rows while the conversations arrive', async () => {
	server.use(graphql.query('WhatsAppConversations', () => new Promise(() => {})))
	renderPluginAt(plugin, '/whatsapp', { session: adminSession })

	const status = await screen.findByRole('status')
	expect(status).toHaveTextContent('Loading conversations…')
	expect(status.closest('.godmin-loading-rows')).not.toBeNull()
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
