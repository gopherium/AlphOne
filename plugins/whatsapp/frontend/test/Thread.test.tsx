// SPDX-License-Identifier: AGPL-3.0-or-later

import { GraphProvider } from '@alphone/frontend-sdk'
import { HttpResponse, fakeGraphClient, graphql, server } from '@alphone/frontend-sdk/testing'
import type { FakeGraph } from '@alphone/frontend-sdk/testing'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { fireEvent, render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { afterEach, beforeEach, expect, test, vi } from 'vitest'

import { handlers } from '../handlers'
import { Thread } from '../Thread'

const conversationId = '019f4a00-0000-7000-8000-000000000001'

beforeEach(() => server.use(...handlers))
afterEach(() => vi.restoreAllMocks())

/** Builds one graph message, defaulting to an inbound text. */
function message(fields: Record<string, unknown> = {}) {
	return {
		__typename: 'WhatsAppMessage',
		id: '019f4a00-0000-7000-8000-0000000000b1',
		externalId: 'wamid.HBgLMTU1NTAwMDExMQ',
		direction: 'inbound',
		content: 'Hi, is the order ready?',
		contentType: 'text',
		sentAt: '2026-07-06T10:00:00Z',
		status: null,
		statusDetail: null,
		media: null,
		...fields,
	}
}

/** Serves the thread document with the given messages. */
function threadOf(...messages: Record<string, unknown>[]) {
	server.use(
		graphql.query('WhatsAppThread', () =>
			HttpResponse.json({
				data: {
					whatsAppConversation: {
						__typename: 'WhatsAppConversation',
						id: conversationId,
						contact: {
							__typename: 'Contact',
							id: '019f4a00-0000-7000-8000-0000000000a1',
							name: 'John Doe',
						},
						messages,
					},
				},
			}),
		),
	)
}

/** Renders the thread under a fresh query client and a drivable graph client. */
function renderThread(): FakeGraph {
	const client = new QueryClient({
		defaultOptions: { queries: { retry: false } },
	})
	const fake = fakeGraphClient()
	render(
		<QueryClientProvider client={client}>
			<GraphProvider graph={fake.graph}>
				<Thread conversationId={conversationId} />
			</GraphProvider>
		</QueryClientProvider>,
	)
	return fake
}

test('ghosts the log while the messages arrive', async () => {
	server.use(graphql.query('WhatsAppThread', () => new Promise(() => {})))
	renderThread()

	const status = await screen.findByRole('status')
	expect(status).toHaveTextContent('Loading messages…')
	expect(status.closest('.godmin-loading-rows')).not.toBeNull()
})

test('names the contact the conversation belongs to in its header', async () => {
	renderThread()

	expect(
		await screen.findByRole('heading', { level: 1, name: 'John Doe' }),
	).toBeInTheDocument()
})

test('titles the header generically while the conversation is unknown', async () => {
	server.use(
		graphql.query('WhatsAppThread', () =>
			HttpResponse.json({ data: { whatsAppConversation: null } }),
		),
	)
	renderThread()

	expect(
		await screen.findByRole('heading', { level: 1, name: 'Conversation' }),
	).toBeInTheDocument()
})

test('lists the conversation messages, oldest first', async () => {
	renderThread()

	expect(await screen.findByText('Hi, is the order ready?')).toBeInTheDocument()
	expect(screen.getByText('I can pick it up after 5pm.')).toBeInTheDocument()

	const contents = screen
		.getAllByRole('listitem')
		.map((item) => item.textContent)
	expect(contents[0]).toContain('Jul 6, 2026')
	expect(contents[1]).toContain('Hi, is the order ready?')
	expect(contents[2]).toContain('I can pick it up after 5pm.')
	expect(screen.getByText('Jul 6, 2026').closest('time')).toHaveAttribute(
		'datetime',
		'2026-07-06',
	)

	const log = screen.getByRole('log', { name: 'Messages' })
	expect(log).toHaveAttribute('tabindex', '0')
	expect(screen.getAllByText('Received')).toHaveLength(2)
	expect(screen.getByText('09:05')).toBeInTheDocument()
	expect(screen.getByText('10:05')).toBeInTheDocument()
})

test('shows an empty state when the conversation has no messages', async () => {
	threadOf()

	renderThread()

	expect(await screen.findByText(/no messages yet/i)).toBeInTheDocument()
})

test('reports when messages cannot be loaded', async () => {
	server.use(
		graphql.query('WhatsAppThread', () =>
			HttpResponse.json({ data: null, errors: [{ message: 'internal error' }] }),
		),
	)

	renderThread()

	expect(await screen.findByRole('alert')).toHaveTextContent(/could not be loaded/i)
})

test('sends a reply and shows it in the thread', async () => {
	const user = userEvent.setup()
	server.use(
		graphql.query('WhatsAppThread', () =>
			HttpResponse.json({
				data: {
					whatsAppConversation: {
						__typename: 'WhatsAppConversation',
						id: conversationId,
						contact: { __typename: 'Contact', id: 'contact-id', name: 'John Doe' },
						messages: [message()],
					},
				},
			}),
		),
		graphql.mutation('WhatsAppSendMessage', ({ variables }) =>
			HttpResponse.json({
				data: {
					whatsAppSendMessage: message({
						id: '019f4a00-0000-7000-8000-0000000000c1',
						externalId: 'wamid.out.1',
						direction: 'outbound',
						content: variables.content,
						sentAt: '2026-07-07T18:00:00Z',
						status: 'sent',
					}),
				},
			}),
		),
	)
	renderThread()
	await screen.findByText('Hi, is the order ready?')

	await user.type(screen.getByRole('textbox', { name: /reply/i }), 'Ready at 5pm')
	await user.click(screen.getByRole('button', { name: /send/i }))

	expect(await screen.findByText('Ready at 5pm')).toBeInTheDocument()
	expect(screen.getByText('Message sent')).toBeInTheDocument()
	expect(screen.getByText('Jul 7, 2026')).toBeInTheDocument()
	expect(screen.getByRole('textbox', { name: /reply/i })).toHaveValue('')
})

test('appends an arriving message without refetching the thread', async () => {
	let threadReads = 0
	server.use(
		graphql.query('WhatsAppThread', () => {
			threadReads += 1
			return HttpResponse.json({
				data: {
					whatsAppConversation: {
						__typename: 'WhatsAppConversation',
						id: conversationId,
						contact: { __typename: 'Contact', id: 'contact-id', name: 'John Doe' },
						messages: [message()],
					},
				},
			})
		}),
	)
	const fake = renderThread()
	await screen.findByText('Hi, is the order ready?')
	const readsBeforeArrival = threadReads

	fake.emit({
		whatsAppMessageReceived: message({
			id: '019f4a00-0000-7000-8000-0000000000e1',
			externalId: 'wamid.in.2',
			content: 'One more thing',
			sentAt: '2026-07-06T10:30:00Z',
		}),
	})

	expect(await screen.findByText('One more thing')).toBeInTheDocument()
	expect(threadReads).toBe(readsBeforeArrival)
	expect(fake.documents[0]).toContain('whatsAppMessageReceived')
})

test('keeps one copy of an arrival the thread reload also returns', async () => {
	const arrival = message({
		id: '019f4a00-0000-7000-8000-0000000000e1',
		content: 'One more thing',
		sentAt: '2026-07-06T10:30:00Z',
	})
	threadOf(message(), arrival)
	const fake = renderThread()
	await screen.findByText('Hi, is the order ready?')

	fake.emit({ whatsAppMessageReceived: arrival })

	expect(await screen.findAllByText('One more thing')).toHaveLength(1)
})

test('reports when the reply cannot be sent', async () => {
	const user = userEvent.setup()
	server.use(
		graphql.mutation('WhatsAppSendMessage', () =>
			HttpResponse.json({
				data: null,
				errors: [{ message: 'upstream failure', extensions: { code: 'UPSTREAM' } }],
			}),
		),
	)

	renderThread()
	await screen.findByText('Hi, is the order ready?')

	await user.type(screen.getByRole('textbox', { name: /reply/i }), 'hello')
	await user.click(screen.getByRole('button', { name: /send/i }))

	expect(await screen.findByText(/could not be sent/i)).toBeInTheDocument()
	expect(screen.getByRole('textbox', { name: /reply/i })).toHaveValue('hello')
})

test('explains a rejected reply with the code the graph surfaced', async () => {
	const user = userEvent.setup()
	server.use(
		graphql.mutation('WhatsAppSendMessage', () =>
			HttpResponse.json({
				data: null,
				errors: [
					{
						message: 'outside the window',
						extensions: { code: 'UPSTREAM', metaCode: 131047 },
					},
				],
			}),
		),
	)

	renderThread()
	await screen.findByText('Hi, is the order ready?')

	await user.type(screen.getByRole('textbox', { name: /reply/i }), 'hello')
	await user.click(screen.getByRole('button', { name: /send/i }))

	expect(await screen.findByRole('alert')).toHaveTextContent(/24-hour/i)
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

test('follows the newest message unless the reader scrolls up', async () => {
	vi.spyOn(Element.prototype, 'scrollHeight', 'get').mockReturnValue(400)
	vi.spyOn(Element.prototype, 'clientHeight', 'get').mockReturnValue(100)
	const scrollTopGet = vi
		.spyOn(Element.prototype, 'scrollTop', 'get')
		.mockReturnValue(0)
	const scrollTopSet = vi
		.spyOn(Element.prototype, 'scrollTop', 'set')
		.mockImplementation(() => {})
	threadOf(message())
	const fake = renderThread()
	await screen.findByText('Hi, is the order ready?')
	expect(scrollTopSet).toHaveBeenCalledTimes(1)
	expect(scrollTopSet).toHaveBeenLastCalledWith(400)

	const log = screen.getByRole('log')
	fireEvent.scroll(log)
	fake.emit({
		whatsAppMessageReceived: message({ id: 'first-id', content: 'first' }),
	})
	await screen.findByText('first')
	expect(scrollTopSet).toHaveBeenCalledTimes(1)

	scrollTopGet.mockReturnValue(350)
	fireEvent.scroll(log)
	fake.emit({
		whatsAppMessageReceived: message({ id: 'second-id', content: 'second' }),
	})
	await screen.findByText('second')
	expect(scrollTopSet).toHaveBeenCalledTimes(2)
})
