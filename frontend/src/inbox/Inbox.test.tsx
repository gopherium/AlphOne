// SPDX-License-Identifier: AGPL-3.0-or-later

import { screen } from '@testing-library/react'
import { HttpResponse, http } from 'msw'
import { expect, test } from 'vitest'

import { renderAt } from '../test/render'
import { server } from '../test/setup'

test('lists conversations from the API, most recent first', async () => {
	renderAt('/')

	expect(await screen.findByText('John Doe')).toBeInTheDocument()
	expect(screen.getByText('María Pérez')).toBeInTheDocument()

	const names = screen.getAllByRole('listitem').map((item) => item.textContent)
	expect(names[0]).toContain('John Doe')
	expect(names[1]).toContain('María Pérez')
})

test('shows an empty state when no conversations exist', async () => {
	server.use(
		http.get('/api/plugins/whatsapp/conversations', () =>
			HttpResponse.json([]),
		),
	)

	renderAt('/')

	expect(await screen.findByText(/no conversations yet/i)).toBeInTheDocument()
})

test('reports when conversations cannot be loaded', async () => {
	server.use(
		http.get('/api/plugins/whatsapp/conversations', () =>
			HttpResponse.json({ error: 'internal error' }, { status: 500 }),
		),
	)

	renderAt('/')

	expect(await screen.findByText(/could not be loaded/i)).toBeInTheDocument()
})
