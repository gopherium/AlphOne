// SPDX-License-Identifier: AGPL-3.0-or-later

import { server } from '@alphone/frontend-sdk/testing'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { HttpResponse, http } from 'msw'
import { beforeEach, expect, test } from 'vitest'

import { handlers } from '../handlers'
import { Thread } from '../Thread'

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

test('sends a reply and appends it to the thread', async () => {
	const user = userEvent.setup()
	let thread = [
		{
			id: '019f4a00-0000-7000-8000-0000000000b1',
			external_id: 'wamid.HBgLMTU1NTAwMDExMQ',
			direction: 'inbound',
			content: 'Hi, is the order ready?',
			content_type: 'text',
			sent_at: '2026-07-06T10:00:00Z',
		},
	]
	server.use(
		http.get(
			'/api/plugins/whatsapp/conversations/:conversationId/messages',
			() => HttpResponse.json(thread),
		),
		http.post(
			'/api/plugins/whatsapp/conversations/:conversationId/messages',
			async ({ request }) => {
				const body = (await request.json()) as { content: string }
				const message = {
					id: '019f4a00-0000-7000-8000-0000000000c1',
					external_id: 'wamid.out.1',
					direction: 'outbound',
					content: body.content,
					content_type: 'text',
					sent_at: '2026-07-07T18:00:00Z',
				}
				thread = [...thread, message]
				return HttpResponse.json(message, { status: 201 })
			},
		),
	)

	renderThread()
	await screen.findByText('Hi, is the order ready?')

	await user.type(
		screen.getByRole('textbox', { name: /reply/i }),
		'Ready at 5pm',
	)
	await user.click(screen.getByRole('button', { name: /send/i }))

	expect(await screen.findByText('Ready at 5pm')).toBeInTheDocument()
	expect(screen.getByText('outbound')).toBeInTheDocument()
	expect(screen.getByRole('textbox', { name: /reply/i })).toHaveValue('')
})

test('reports when the reply cannot be sent', async () => {
	const user = userEvent.setup()
	server.use(
		http.post(
			'/api/plugins/whatsapp/conversations/:conversationId/messages',
			() => HttpResponse.json({ error: 'upstream failure' }, { status: 502 }),
		),
	)

	renderThread()
	await screen.findByText('Hi, is the order ready?')

	await user.type(screen.getByRole('textbox', { name: /reply/i }), 'hello')
	await user.click(screen.getByRole('button', { name: /send/i }))

	expect(await screen.findByText(/could not be sent/i)).toBeInTheDocument()
})

test('refuses to send a blank reply', async () => {
	const user = userEvent.setup()

	renderThread()
	await screen.findByText('Hi, is the order ready?')

	const send = screen.getByRole('button', { name: /send/i })
	expect(send).toHaveAttribute('aria-disabled', 'true')

	await user.type(screen.getByRole('textbox', { name: /reply/i }), '   ')
	await user.click(send)

	expect(send).toHaveAttribute('aria-disabled', 'true')
})
