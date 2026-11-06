// SPDX-License-Identifier: AGPL-3.0-or-later

import { GraphProvider, Toaster, configureErrorText } from '@alphone/frontend-sdk'
import { HttpResponse, fakeGraphClient, graphql, server } from '@alphone/frontend-sdk/testing'
import type { FakeGraph } from '@alphone/frontend-sdk/testing'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { act, fireEvent, render, screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { StrictMode } from 'react'
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

/** Builds the conversation of the thread document, its oldest message the first one given unless named. */
function conversationOf(
	number: string | null,
	messages: Record<string, unknown>[],
	oldestMessageId: unknown = messages[0]?.id ?? null,
) {
	return {
		__typename: 'WhatsAppConversation',
		id: conversationId,
		number,
		oldestMessageId,
		contact: {
			__typename: 'Contact',
			id: '019f4a00-0000-7000-8000-0000000000a1',
			name: 'John Doe',
		},
		messages,
	}
}

/** Serves the thread document of a conversation on a number with the given messages. */
function threadOn(number: string | null, ...messages: Record<string, unknown>[]) {
	server.use(
		graphql.query('WhatsAppThread', () =>
			HttpResponse.json({ data: { whatsAppConversation: conversationOf(number, messages) } }),
		),
	)
}

/** Builds message n of a long chat, sent n minutes past ten. */
function chatMessage(n: number, fields: Record<string, unknown> = {}) {
	const two = String(n).padStart(2, '0')
	return message({
		id: `019f4a00-0000-7000-8000-0000000001${two}`,
		externalId: `wamid.chat.${two}`,
		content: `Message ${n}`,
		sentAt: `2026-07-06T10:${two}:00Z`,
		...fields,
	})
}

/** Builds the first n messages of a long chat, oldest first. */
function chatOf(n: number) {
	return Array.from({ length: n }, (_, index) => chatMessage(index + 1))
}

/** Serves a chat two messages at a time from the chat as it stands, an older page only once the gate opens. */
function chatInPagesOfTwo(chat: Record<string, unknown>[], gate: () => Promise<void> = async () => {}) {
	server.use(
		graphql.query('WhatsAppThread', async ({ variables }) => {
			if (variables.after) {
				await gate()
			}
			const end = variables.after ? chat.findIndex((row) => row.id === variables.after) : chat.length
			const page = chat.slice(Math.max(0, end - 2), end)
			return HttpResponse.json({ data: { whatsAppConversation: conversationOf(null, page, chat[0]?.id) } })
		}),
	)
}

/** Returns a gate that holds every read until it opens. */
function heldGate() {
	let open: () => void = () => {}
	const opened = new Promise<void>((resolve) => {
		open = resolve
	})
	return { wait: () => opened, open: () => open() }
}

/** Returns the text of every row in the log, in order. */
function rowTexts(): string[] {
	return screen.getAllByRole('listitem').map((item) => item.textContent ?? '')
}

/** OLDER names the button that loads the older messages. */
const OLDER = { name: 'Show older messages' }

/** Lists one message, delivers an arrival, and returns the row each one is listed on. */
async function rowsAfterArrival(loaded: ReturnType<typeof message>, arrival: ReturnType<typeof message>) {
	threadOf(loaded)
	const fake = renderThread()
	await screen.findByText(loaded.content)

	fake.emit({ whatsAppMessageReceived: arrival })
	await screen.findByText(arrival.content)

	const contents = rowTexts()
	return {
		loaded: contents.findIndex((text) => text.includes(loaded.content)),
		arrival: contents.findIndex((text) => text.includes(arrival.content)),
	}
}

/** Loads the older page of a long chat whose log reports the given metrics, and returns the spy on each scroll set. */
async function loadOlderMessages({
	clientHeight,
	scrollTop,
	scrolled,
	scrolledTo,
}: {
	clientHeight: number
	scrollTop: number
	scrolled: boolean
	scrolledTo?: number
}) {
	const user = userEvent.setup()
	let height = 1000
	vi.spyOn(Element.prototype, 'scrollHeight', 'get').mockImplementation(() => height)
	vi.spyOn(Element.prototype, 'clientHeight', 'get').mockReturnValue(clientHeight)
	const scrollTopGet = vi.spyOn(Element.prototype, 'scrollTop', 'get').mockReturnValue(scrollTop)
	const scrollTopSet = vi.spyOn(Element.prototype, 'scrollTop', 'set').mockImplementation(() => {})
	const gate = heldGate()
	chatInPagesOfTwo(chatOf(4), gate.wait)
	renderThread()
	const button = await screen.findByRole('button', OLDER)
	const log = screen.getByRole('log')
	if (scrolled) {
		fireEvent.scroll(log)
	}
	scrollTopSet.mockClear()

	await user.click(button)
	await waitFor(() => expect(button).toHaveAttribute('aria-disabled', 'true'))
	if (scrolledTo !== undefined) {
		scrollTopGet.mockReturnValue(scrolledTo)
		fireEvent.scroll(log)
	}
	height = 1600
	gate.open()
	await screen.findByText('Message 1')
	return scrollTopSet
}

/** Serves the thread document with the given messages. */
function threadOf(...messages: Record<string, unknown>[]) {
	threadOn(null, ...messages)
}

/** Renders the thread under a fresh query client and a drivable graph client, in StrictMode when asked. */
function renderThread({ strict = false }: { strict?: boolean } = {}): FakeGraph {
	const client = new QueryClient({
		defaultOptions: { queries: { retry: false } },
	})
	const fake = fakeGraphClient([plugin])
	const tree = (
		<QueryClientProvider client={client}>
			<GraphProvider graph={fake.graph}>
				<Toaster>
					<Thread conversationId={conversationId} />
				</Toaster>
			</GraphProvider>
		</QueryClientProvider>
	)
	render(strict ? <StrictMode>{tree}</StrictMode> : tree)
	return fake
}

/** upstreamFailure answers every send with a failure the provider reported. */
const upstreamFailure = graphql.mutation('WhatsAppSendMessage', () =>
	HttpResponse.json({
		data: null,
		errors: [{ message: 'upstream failure', extensions: { code: 'UPSTREAM' } }],
	}),
)

/** Waits for the first message, then types hello into the composer and sends it. */
async function sendReply() {
	const user = userEvent.setup()
	await screen.findByText('Hi, is the order ready?')
	await user.type(screen.getByRole('textbox', { name: /reply/i }), 'hello')
	await user.click(screen.getByRole('button', { name: /send/i }))
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
	expect(screen.queryByRole('button', OLDER)).toBeNull()
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

test('shows the alert and no composer when the first read answers a null conversation with errors', async () => {
	server.use(
		graphql.query('WhatsAppThread', () =>
			HttpResponse.json({
				data: { whatsAppConversation: null },
				errors: [{ message: 'internal error', path: ['whatsAppConversation', 'messages'] }],
			}),
		),
	)

	renderThread()

	expect(await screen.findByRole('alert')).toHaveTextContent('Messages could not be loaded.')
	expect(screen.queryByRole('textbox', { name: /reply/i })).toBeNull()
})

test('sends a reply and shows it in the thread', async () => {
	const user = userEvent.setup()
	server.use(
		graphql.query('WhatsAppThread', () =>
			HttpResponse.json({ data: { whatsAppConversation: conversationOf(null, [message()]) } }),
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
			return HttpResponse.json({ data: { whatsAppConversation: conversationOf(null, [message()]) } })
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
	const rows = await rowsAfterArrival(
		message({
			id: '019f4a00-0000-7000-8000-0000000000b2',
			content: 'Later message',
			sentAt: '2026-07-06T10:30:00Z',
		}),
		message({
			id: '019f4a00-0000-7000-8000-0000000000b1',
			content: 'Earlier message',
			sentAt: '2026-07-06T10:00:00Z',
		}),
	)

	expect(rows.arrival).toBeLessThan(rows.loaded)
})

test('lists two messages sent in the same second by their ids', async () => {
	const rows = await rowsAfterArrival(
		message({
			id: '019f4a00-0000-7000-8000-0000000000b2',
			content: 'Higher id',
			sentAt: '2026-07-06T10:00:00Z',
		}),
		message({
			id: '019f4a00-0000-7000-8000-0000000000b1',
			content: 'Lower id',
			sentAt: '2026-07-06T10:00:00Z',
		}),
	)

	expect(rows.arrival).toBeLessThan(rows.loaded)
})

test('reports when the reply cannot be sent', async () => {
	server.use(upstreamFailure)

	renderThread()
	await sendReply()

	expect(await screen.findByText(/could not be sent/i)).toBeInTheDocument()
	expect(screen.getByRole('textbox', { name: /reply/i })).toHaveValue('hello')
})

test('keeps a reply that fails on screen in the composer under StrictMode, with no toast', async () => {
	server.use(upstreamFailure)

	renderThread({ strict: true })
	await sendReply()

	expect(await screen.findByRole('alert')).toHaveTextContent(/could not be sent/i)
	expect(screen.getByRole('textbox', { name: /reply/i })).toHaveValue('hello')
	expect(screen.getAllByText(/could not be sent/i)).toHaveLength(1)
})

test('says the number is gone when a reply meets a removed number', async () => {
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
	await sendReply()

	expect(await screen.findByRole('alert')).toHaveTextContent('That number is no longer connected.')
	expect(screen.getByRole('textbox', { name: /reply/i })).toHaveValue('hello')
})

test('explains a rejected reply with the code the graph surfaced', async () => {
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
	await sendReply()

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

test('hides the older messages button when the first page holds the oldest message', async () => {
	chatInPagesOfTwo(chatOf(2))
	renderThread()

	await screen.findByText('Message 1')

	expect(screen.queryByRole('button', OLDER)).toBeNull()
})

test('shows the older messages button while older messages remain', async () => {
	chatInPagesOfTwo(chatOf(3))
	renderThread()

	expect(await screen.findByRole('button', OLDER)).toBeInTheDocument()
	expect(screen.queryByText('Message 1')).toBeNull()
})

test('adds the older page above the rows already shown', async () => {
	const user = userEvent.setup()
	chatInPagesOfTwo(chatOf(4))
	renderThread()

	await user.click(await screen.findByRole('button', OLDER))
	await screen.findByText('Message 1')

	expect(rowTexts().filter((text) => text.startsWith('Received'))).toEqual([
		expect.stringContaining('Message 1'),
		expect.stringContaining('Message 2'),
		expect.stringContaining('Message 3'),
		expect.stringContaining('Message 4'),
	])
})

test('hides the button once the oldest message is loaded', async () => {
	const user = userEvent.setup()
	chatInPagesOfTwo(chatOf(6))
	renderThread()

	await user.click(await screen.findByRole('button', OLDER))
	await screen.findByText('Message 3')
	expect(screen.getByRole('button', OLDER)).toBeInTheDocument()
	await user.click(screen.getByRole('button', OLDER))
	await screen.findByText('Message 1')

	expect(screen.queryByRole('button', OLDER)).toBeNull()
})

test('keeps the reader on the same row when older messages load', async () => {
	const scrollTopSet = await loadOlderMessages({ clientHeight: 100, scrollTop: 0, scrolled: true })

	expect(scrollTopSet).toHaveBeenLastCalledWith(600)
})

test('does not jump to the newest message when older messages load while following', async () => {
	const scrollTopSet = await loadOlderMessages({ clientHeight: 900, scrollTop: 50, scrolled: false })

	expect(scrollTopSet).toHaveBeenLastCalledWith(650)
})

test('keeps the row the reader scrolls to while older messages load', async () => {
	const scrollTopSet = await loadOlderMessages({ clientHeight: 100, scrollTop: 0, scrolled: true, scrolledTo: 300 })

	expect(scrollTopSet).toHaveBeenLastCalledWith(900)
})

test('lists each row once when new messages move the first page and an older page repeats an arrival', async () => {
	const user = userEvent.setup()
	const chat = chatOf(4)
	chatInPagesOfTwo(chat)
	const fake = renderThread()
	await screen.findByText('Message 4')

	for (const n of [5, 6, 7]) {
		chat.push(chatMessage(n))
		fake.emit({ whatsAppMessageReceived: chatMessage(n) })
	}
	act(() => fake.graph.refetch(['WhatsAppThread']))
	await waitFor(() => expect(screen.queryByText('Message 4')).toBeNull())
	await user.click(screen.getByRole('button', OLDER))
	await screen.findByText('Message 4')

	for (const n of [4, 5, 6, 7]) {
		expect(screen.getAllByText(`Message ${n}`)).toHaveLength(1)
	}
	expect(rowTexts().filter((text) => text.startsWith('Received'))).toEqual([
		expect.stringContaining('Message 4'),
		expect.stringContaining('Message 5'),
		expect.stringContaining('Message 6'),
		expect.stringContaining('Message 7'),
	])
})

test('moves the ticks on an older row when a conversation event rereads the pages', async () => {
	const user = userEvent.setup()
	const chat = chatOf(4)
	chat[0] = chatMessage(1, { direction: 'outbound', status: 'sent' })
	chatInPagesOfTwo(chat)
	const fake = renderThread()
	await user.click(await screen.findByRole('button', OLDER))
	expect(await screen.findByText('Message sent')).toBeInTheDocument()

	chat[0] = chatMessage(1, { direction: 'outbound', status: 'delivered' })
	act(() => fake.graph.refetch(['WhatsAppThread']))

	expect(await screen.findByText('Message delivered')).toBeInTheDocument()
	expect(screen.queryByText('Message sent')).toBeNull()
})

test('keeps the oldest loaded row listed when a new message rereads the pages', async () => {
	const user = userEvent.setup()
	const chat = chatOf(4)
	chat[1] = chatMessage(2, { direction: 'outbound', status: 'sent' })
	chatInPagesOfTwo(chat)
	const fake = renderThread()
	await user.click(await screen.findByRole('button', OLDER))
	await screen.findByText('Message 1')

	chat[1] = chatMessage(2, { direction: 'outbound', status: 'delivered' })
	chat.push(chatMessage(5))
	act(() => fake.graph.refetch(['WhatsAppThread']))
	await screen.findByText('Message delivered')

	expect(screen.getAllByText(/^Message \d$/).map((node) => node.textContent)).toEqual([
		'Message 1',
		'Message 2',
		'Message 3',
		'Message 4',
		'Message 5',
	])
	expect(screen.queryByRole('button', OLDER)).toBeNull()
})

test('shows a notice when an older page cannot be reached and keeps the log and the composer', async () => {
	const user = userEvent.setup()
	chatInPagesOfTwo(chatOf(4))
	server.use(
		graphql.query('WhatsAppThread', ({ variables }) => (variables.after ? HttpResponse.error() : undefined)),
	)
	renderThread()

	await user.click(await screen.findByRole('button', OLDER))

	expect(await screen.findByRole('alert')).toHaveTextContent('Messages could not be loaded.')
	expect(within(screen.getByRole('log')).queryByRole('alert')).toBeNull()
	expect(screen.getByText('Message 3')).toBeInTheDocument()
	expect(screen.getByRole('textbox', { name: /reply/i })).toBeInTheDocument()
})
