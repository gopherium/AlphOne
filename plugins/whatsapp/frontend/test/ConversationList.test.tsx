// SPDX-License-Identifier: AGPL-3.0-or-later

import { HttpResponse, graphql, renderPluginAt, server } from '@alphone/frontend-sdk/testing'
import { screen } from '@testing-library/react'
import { beforeEach, expect, test } from 'vitest'

import { handlers } from '../handlers'
import { plugin } from '../index'

beforeEach(() => server.use(...handlers))

test('ghosts the rail rows while the conversations arrive', async () => {
	server.use(graphql.query('WhatsAppConversations', () => new Promise(() => {})))
	renderPluginAt(plugin, '/whatsapp')

	const status = await screen.findByRole('status')
	expect(status).toHaveTextContent('Loading conversations…')
	expect(status.closest('.godmin-loading-rows')).not.toBeNull()
})

test('lists conversations from the graph, most recent first', async () => {
	renderPluginAt(plugin, '/whatsapp')

	expect(await screen.findByText('John Doe')).toBeInTheDocument()
	expect(screen.getByText('María Pérez')).toBeInTheDocument()

	const names = screen.getAllByRole('listitem').map((item) => item.textContent)
	expect(names[0]).toContain('John Doe')
	expect(names[1]).toContain('María Pérez')
})

test('shows each conversation preview and last activity', async () => {
	renderPluginAt(plugin, '/whatsapp')

	expect(
		await screen.findByText('I can pick it up after 5pm.'),
	).toBeInTheDocument()
	expect(screen.getByText('hello')).toBeInTheDocument()
	expect(screen.getAllByText('Jul 6, 2026')).toHaveLength(2)
	expect(screen.getByText('Jul 5, 2026')).toBeInTheDocument()
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

	renderPluginAt(plugin, '/whatsapp')

	const empty = await screen.findByText(/no conversations yet/i)
	expect(empty).toHaveAttribute('role', 'status')
})

test('reports when conversations cannot be loaded', async () => {
	server.use(
		graphql.query('WhatsAppConversations', () =>
			HttpResponse.json({ data: null, errors: [{ message: 'internal error' }] }),
		),
	)

	renderPluginAt(plugin, '/whatsapp')

	expect(await screen.findByRole('alert')).toHaveTextContent(
		/could not be loaded/i,
	)
})
