import { server } from '@alphone/frontend-sdk/testing'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { render, screen } from '@testing-library/react'
import { HttpResponse, http } from 'msw'
import { beforeEach, expect, test } from 'vitest'

import { handlers } from '../handlers'
import { Thread } from '../Thread'

const messagesPath = '/api/plugins/whatsapp/conversations/:conversationId/messages'

let messageCounter = 0

beforeEach(() => {
	server.use(...handlers)
	messageCounter = 0
})

/**
 * Builds a raw message fixture with unique identifiers.
 * @param overrides - Fields overriding the outbound-text defaults.
 * @returns The raw message payload.
 */
function message(overrides: Record<string, unknown>): Record<string, unknown> {
	messageCounter += 1
	return {
		id: `mid-${messageCounter}`,
		external_id: `wamid.status.${messageCounter}`,
		direction: 'outbound',
		content: 'hola',
		content_type: 'text',
		sent_at: '2026-07-06T09:05:00Z',
		status: null,
		status_detail: null,
		media: null,
		...overrides,
	}
}

/**
 * Serves the given raw messages and renders the thread under test.
 * @param messages - The raw message payloads to return.
 */
function renderThreadOf(...messages: Array<Record<string, unknown>>) {
	server.use(http.get(messagesPath, () => HttpResponse.json(messages)))
	const client = new QueryClient({
		defaultOptions: { queries: { retry: false } },
	})
	render(
		<QueryClientProvider client={client}>
			<Thread conversationId="019f4a00-0000-7000-8000-000000000001" />
		</QueryClientProvider>,
	)
}

/**
 * Returns the rendered tick element, if any.
 * @returns The tick span or null.
 */
function tickElement(): Element | null {
	return document.querySelector('.alphone-message__ticks')
}

test('renders a single tick for sent messages', async () => {
	renderThreadOf(message({ status: 'sent' }))

	expect(await screen.findByText('Message sent')).toBeInTheDocument()
	const tick = tickElement()
	expect(tick).toHaveTextContent('✓')
	expect(tick).not.toHaveTextContent('✓✓')
	expect(tick).not.toHaveClass('alphone-message__ticks--read')
})

test('renders double ticks for delivered messages', async () => {
	renderThreadOf(message({ status: 'delivered' }))

	expect(await screen.findByText('Message delivered')).toBeInTheDocument()
	expect(tickElement()).toHaveTextContent('✓✓')
	expect(tickElement()).not.toHaveClass('alphone-message__ticks--read')
})

test('renders read ticks emphasized', async () => {
	renderThreadOf(message({ status: 'read' }))

	expect(await screen.findByText('Message read')).toBeInTheDocument()
	expect(tickElement()).toHaveClass('alphone-message__ticks--read')
})

test('renders played voice notes like read', async () => {
	renderThreadOf(message({ status: 'played' }))

	expect(await screen.findByText('Message played')).toBeInTheDocument()
	expect(tickElement()).toHaveClass('alphone-message__ticks--read')
})

test('renders failed messages with the mapped explanation', async () => {
	renderThreadOf(message({ status: 'failed', status_detail: '131047 Re-engagement message' }))

	expect(await screen.findByText('Message not delivered')).toBeInTheDocument()
	expect(tickElement()).toHaveClass('alphone-message__ticks--failed')
	expect(
		screen.getByText('Outside the 24-hour window. The customer must message first.'),
	).toBeInTheDocument()
})

test('renders unmapped failure codes with the generic explanation', async () => {
	renderThreadOf(message({ status: 'failed', status_detail: '999 Something strange' }))

	expect(await screen.findByText('Not delivered.')).toBeInTheDocument()
})

test('renders unparsable failure details with the generic explanation', async () => {
	renderThreadOf(message({ status: 'failed', status_detail: 'no code here' }))

	expect(await screen.findByText('Not delivered.')).toBeInTheDocument()
})

test('renders failures without detail with the generic explanation', async () => {
	renderThreadOf(message({ status: 'failed' }))

	expect(await screen.findByText('Not delivered.')).toBeInTheDocument()
})

test('renders no tick before the first status arrives', async () => {
	renderThreadOf(message({ status: null }))

	await screen.findByText('hola')
	expect(tickElement()).not.toBeInTheDocument()
})

test('renders no tick for accepted or unknown statuses', async () => {
	renderThreadOf(message({ status: 'accepted' }))

	await screen.findByText('hola')
	expect(tickElement()).not.toBeInTheDocument()
})

test('renders no tick on inbound messages', async () => {
	renderThreadOf(message({ direction: 'inbound', status: 'delivered' }))

	await screen.findByText('hola')
	expect(tickElement()).not.toBeInTheDocument()
})
