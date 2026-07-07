// SPDX-License-Identifier: AGPL-3.0-or-later

import { server } from '@alphone/frontend-sdk/testing'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { render, screen } from '@testing-library/react'
import { HttpResponse, http } from 'msw'
import { beforeEach, expect, test } from 'vitest'

import { handlers } from './handlers'
import { Thread } from './Thread'

beforeEach(() => server.use(...handlers))

function renderThread() {
	const client = new QueryClient({
		defaultOptions: { queries: { retry: false } },
	})
	render(
		<QueryClientProvider client={client}>
			<Thread conversationId="019f4a00-0000-7000-8000-000000000001" />
		</QueryClientProvider>,
	)
}

test('lists the conversation messages, oldest first', async () => {
	renderThread()

	expect(
		await screen.findByText('Hi, is the order ready?'),
	).toBeInTheDocument()
	expect(screen.getByText('I can pick it up after 5pm.')).toBeInTheDocument()

	const contents = screen
		.getAllByRole('listitem')
		.map((item) => item.textContent)
	expect(contents[0]).toContain('Hi, is the order ready?')
	expect(contents[1]).toContain('I can pick it up after 5pm.')

	expect(screen.getAllByText('inbound')).toHaveLength(2)
})

test('shows an empty state when the conversation has no messages', async () => {
	server.use(
		http.get(
			'/api/plugins/whatsapp/conversations/:conversationId/messages',
			() => HttpResponse.json([]),
		),
	)

	renderThread()

	expect(await screen.findByText(/no messages yet/i)).toBeInTheDocument()
})

test('reports when messages cannot be loaded', async () => {
	server.use(
		http.get(
			'/api/plugins/whatsapp/conversations/:conversationId/messages',
			() => HttpResponse.json({ error: 'internal error' }, { status: 500 }),
		),
	)

	renderThread()

	expect(await screen.findByText(/could not be loaded/i)).toBeInTheDocument()
})
