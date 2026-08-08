// SPDX-License-Identifier: AGPL-3.0-or-later

import { HttpResponse, graphql, server } from '@alphone/frontend-sdk/testing'
import { screen, within } from '@testing-library/react'
import { beforeEach, expect, test } from 'vitest'

import { renderAt } from './render'

beforeEach(() =>
	server.use(
		graphql.query('WhatsAppConversations', () =>
			HttpResponse.json({
				data: {
					whatsAppConversations: [
						{
							__typename: 'WhatsAppConversation',
							id: '019f4a00-0000-7000-8000-000000000001',
							status: 'open',
							lastActivityAt: '2026-07-06T10:05:00Z',
							lastMessagePreview: 'The analytical engine awaits.',
							contact: {
								__typename: 'Contact',
								id: '019f4a00-0000-7000-8000-0000000000a1',
								name: 'Ada Lovelace',
							},
						},
					],
				},
			}),
		),
	),
)

test('drills the sidebar into the WhatsApp section screen', async () => {
	renderAt('/whatsapp')

	expect(
		await screen.findByRole('heading', { name: 'WhatsApp' }),
	).toBeInTheDocument()
	expect(screen.getByRole('link', { name: 'Back' })).toBeInTheDocument()
	expect(await screen.findByText('Ada Lovelace')).toBeInTheDocument()
	expect(
		within(screen.getByRole('main')).getByText('No conversation selected.'),
	).toBeInTheDocument()
	expect(
		screen.getByRole('navigation', { name: 'Navigation' }),
	).toBeInTheDocument()
})

test('shows the main menu, not a section screen, at the root', async () => {
	renderAt('/')

	expect(
		await screen.findByRole('navigation', { name: 'Navigation' }),
	).toBeInTheDocument()
	expect(screen.queryByRole('heading', { name: 'WhatsApp' })).toBeNull()
})
