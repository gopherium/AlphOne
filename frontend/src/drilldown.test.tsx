// SPDX-License-Identifier: AGPL-3.0-or-later

import { HttpResponse, http, server } from '@alphone/frontend-sdk/testing'
import { screen, within } from '@testing-library/react'
import { beforeEach, expect, test } from 'vitest'

import { renderAt } from './test/render'

beforeEach(() =>
	server.use(
		http.get('/api/plugins/whatsapp/conversations', () =>
			HttpResponse.json([
				{
					id: '019f4a00-0000-7000-8000-000000000001',
					contact_id: '019f4a00-0000-7000-8000-0000000000a1',
					contact_name: 'Ada Lovelace',
					external_id: '555000111',
					status: 'open',
					last_activity_at: '2026-07-06T10:05:00Z',
				},
			]),
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
		within(screen.getByRole('main')).getByText(/select a conversation/i),
	).toBeInTheDocument()
})

test('shows the main menu, not a section screen, at the root', async () => {
	renderAt('/')

	expect(
		await screen.findByRole('navigation', { name: 'Main menu' }),
	).toBeInTheDocument()
	expect(screen.queryByRole('heading', { name: 'WhatsApp' })).toBeNull()
})
