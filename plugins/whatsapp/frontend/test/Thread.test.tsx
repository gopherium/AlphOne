// SPDX-License-Identifier: AGPL-3.0-or-later

import { GraphProvider, configureErrorText } from '@alphone/frontend-sdk'
import { HttpResponse, fakeGraphClient, graphql, server } from '@alphone/frontend-sdk/testing'
import type { FakeGraph } from '@alphone/frontend-sdk/testing'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { afterEach, beforeEach, expect, test, vi } from 'vitest'

import { errorTemplates } from '../errorTemplates'
import { handlers, twoNumbers } from '../handlers'
import { plugin } from '../index'
import { Thread } from '../Thread'

const conversationId = '019f4a00-0000-7000-8000-000000000001'

beforeEach(() => server.use(...handlers))
afterEach(() => {
	vi.restoreAllMocks()
	configureErrorText({ templates: () => ({}), fallback: () => '' })
})

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

/** Serves the thread document of a conversation on a number with the given messages. */
function threadOn(number: string | null, ...messages: Record<string, unknown>[]) {
	server.use(
		graphql.query('WhatsAppThread', () =>
			HttpResponse.json({
				data: {
					whatsAppConversation: {
						__typename: 'WhatsAppConversation',
						id: conversationId,
						number,
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

/** Serves the thread document with the given messages. */
function threadOf(...messages: Record<string, unknown>[]) {
	threadOn(null, ...messages)
}

/** Renders the thread under a fresh query client and a drivable graph client. */
function renderThread(): FakeGraph {
	const client = new QueryClient({
		defaultOptions: { queries: { retry: false } },
	})
	const fake = fakeGraphClient([plugin])
	render(
		<QueryClientProvider client={client}>
			<GraphProvider graph={fake.graph}>
				<Thread conversationId={conversationId} />
			</GraphProvider>
		</QueryClientProvider>,
	)
	return fake
}

test('shows a spinner in place of the log while the messages arrive', async () => {
	server.use(graphql.query('WhatsAppThread', () => new Promise(() => {})))
	renderThread()

	const status = await screen.findByRole('status')
	expect(status).toHaveTextContent('Loading messages…')
	expect(status.closest('.godmin-loading-screen')).not.toBeNull()
})

test('names the contact the conversation belongs to in its header', async () => {
	renderThread()

	expect(
		await screen.findByRole('heading', { level: 2, name: 'John Doe' }),
	).toBeInTheDocument()
})

test('names the number in the header once the workspace holds more than one', async () => {
	server.use(twoNumbers)
	threadOn('Sales line', message())
	renderThread()

	expect(await screen.findByText('Number: Sales line')).toHaveClass('alphone-thread__number')
})

test('leaves the number out of the header while the workspace holds one', async () => {
	threadOn('Sales line', message())
	renderThread()

	await screen.findByText('Hi, is the order ready?')

	expect(screen.queryByText(/Number:/)).toBeNull()
})

test('leaves the number out of the header of a conversation on none', async () => {
	let numbersRead = false
	server.use(
		graphql.query('WhatsAppNumbers', () => {
			numbersRead = true
			return HttpResponse.json({
				data: {
					whatsAppNumbers: [
						{ __typename: 'WhatsAppNumber', identifier: '5550001', label: 'Sales line' },
						{ __typename: 'WhatsAppNumber', identifier: '5550002', label: '' },
					],
				},
			})
		}),
	)
	threadOn(null, message())
	renderThread()

	await screen.findByText('Hi, is the order ready?')
	await waitFor(() => expect(numbersRead).toBe(true))

	expect(screen.queryByText(/Number:/)).toBeNull()
})

test('titles the header generically while the conversation is unknown', async () => {
	server.use(
		graphql.query('WhatsAppThread', () =>
			HttpResponse.json({ data: { whatsAppConversation: null } }),
		),
	)
	renderThread()

	expect(
		await screen.findByRole('heading', { level: 2, name: 'Conversation' }),
	).toBeInTheDocument()
})

test('lists the conversation messages, oldest first', async () => {
	renderThread()

	expect(await screen.findByText('Hi, is the order ready?')).toBeInTheDocument()
	expect(screen.getByText('I can pick it up after 5pm.')).toBeInTheDocument()

	const contents = screen
		.getAllByRole('listitem')
		.map((item) => item.textContent)
	expect(contents[0]).toContain('06/07/2026')
	expect(contents[1]).toContain('Hi, is the order ready?')
	expect(contents[2]).toContain('I can pick it up after 5pm.')
	expect(screen.getByText('06/07/2026').closest('time')).toHaveAttribute(
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
						number: null,
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
	expect(screen.getByText('07/07/2026')).toBeInTheDocument()
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
						number: null,
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

test('lists an arrival older than the loaded rows in its sent order', async () => {
	threadOf(
		message({
			id: '019f4a00-0000-7000-8000-0000000000b2',
			content: 'Later message',
			sentAt: '2026-07-06T10:30:00Z',
		}),
	)
	const fake = renderThread()
	await screen.findByText('Later message')

	fake.emit({
		whatsAppMessageReceived: message({
			id: '019f4a00-0000-7000-8000-0000000000b1',
			content: 'Earlier message',
			sentAt: '2026-07-06T10:00:00Z',
		}),
	})
	await screen.findByText('Earlier message')

	const contents = screen.getAllByRole('listitem').map((item) => item.textContent ?? '')
	expect(contents.findIndex((text) => text.includes('Earlier message'))).toBeLessThan(
		contents.findIndex((text) => text.includes('Later message')),
	)
})

test('lists two messages sent in the same second by their ids', async () => {
	threadOf(
		message({
			id: '019f4a00-0000-7000-8000-0000000000b2',
			content: 'Higher id',
			sentAt: '2026-07-06T10:00:00Z',
		}),
	)
	const fake = renderThread()
	await screen.findByText('Higher id')

	fake.emit({
		whatsAppMessageReceived: message({
			id: '019f4a00-0000-7000-8000-0000000000b1',
			content: 'Lower id',
			sentAt: '2026-07-06T10:00:00Z',
		}),
	})
	await screen.findByText('Lower id')

	const contents = screen.getAllByRole('listitem').map((item) => item.textContent ?? '')
	expect(contents.findIndex((text) => text.includes('Lower id'))).toBeLessThan(
		contents.findIndex((text) => text.includes('Higher id')),
	)
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

test('says the number is gone when a reply meets a removed number', async () => {
	const user = userEvent.setup()
	configureErrorText({ templates: errorTemplates, fallback: () => '' })
	server.use(
		graphql.mutation('WhatsAppSendMessage', () =>
			HttpResponse.json({
				data: null,
				errors: [
					{
						message: 'whatsapp: that number is no longer connected',
						extensions: { code: 'VALIDATION', reason: 'number_removed' },
					},
				],
			}),
		),
	)

	renderThread()
	await screen.findByText('Hi, is the order ready?')

	await user.type(screen.getByRole('textbox', { name: /reply/i }), 'hello')
	await user.click(screen.getByRole('button', { name: /send/i }))

	expect(await screen.findByRole('alert')).toHaveTextContent('That number is no longer connected.')
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
