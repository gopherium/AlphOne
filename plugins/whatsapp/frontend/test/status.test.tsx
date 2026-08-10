// SPDX-License-Identifier: AGPL-3.0-or-later

import { GraphProvider } from '@alphone/frontend-sdk'
import { fakeGraphClient, server } from '@alphone/frontend-sdk/testing'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { HttpResponse, graphql } from 'msw'
import { beforeEach, expect, test } from 'vitest'

import { handlers } from '../handlers'
import { Thread } from '../Thread'

const conversationId = '019f4a00-0000-7000-8000-000000000001'

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
		__typename: 'WhatsAppMessage',
		id: `mid-${messageCounter}`,
		externalId: `wamid.status.${messageCounter}`,
		direction: 'outbound',
		content: 'hello',
		contentType: 'text',
		sentAt: '2026-07-06T09:05:00Z',
		status: null,
		statusDetail: null,
		media: null,
		...overrides,
	}
}

/**
 * Serves the given raw messages and renders the thread under test.
 * @param messages - The raw message payloads to return.
 */
function renderThreadOf(...messages: Array<Record<string, unknown>>) {
	server.use(
		graphql.query('WhatsAppThread', () =>
			HttpResponse.json({
				data: {
					whatsAppConversation: {
						__typename: 'WhatsAppConversation',
						id: conversationId,
						contact: { __typename: 'Contact', id: 'contact-id', name: 'John Doe' },
						messages,
					},
				},
			}),
		),
	)
	const client = new QueryClient({
		defaultOptions: { queries: { retry: false } },
	})
	const { graph } = fakeGraphClient()
	render(
		<QueryClientProvider client={client}>
			<GraphProvider graph={graph}>
				<Thread conversationId={conversationId} />
			</GraphProvider>
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
	renderThreadOf(message({ status: 'failed', statusDetail: '131047 Re-engagement message' }))

	expect(await screen.findByText('Message not delivered')).toBeInTheDocument()
	expect(tickElement()).toHaveClass('alphone-message__ticks--failed')
	expect(
		screen.getByText('Outside the 24-hour window. The customer must message first.'),
	).toBeInTheDocument()
})

test('renders unmapped failure codes with the generic explanation', async () => {
	renderThreadOf(message({ status: 'failed', statusDetail: '999 Something strange' }))

	expect(await screen.findByText('Not delivered.')).toBeInTheDocument()
})

test('renders unparsable failure details with the generic explanation', async () => {
	renderThreadOf(message({ status: 'failed', statusDetail: 'no code here' }))

	expect(await screen.findByText('Not delivered.')).toBeInTheDocument()
})

test('renders failures without detail with the generic explanation', async () => {
	renderThreadOf(message({ status: 'failed' }))

	expect(await screen.findByText('Not delivered.')).toBeInTheDocument()
})

test('renders no tick before the first status arrives', async () => {
	renderThreadOf(message({ status: null }))

	await screen.findByText('hello')
	expect(tickElement()).not.toBeInTheDocument()
})

test('renders no tick for accepted or unknown statuses', async () => {
	renderThreadOf(message({ status: 'accepted' }))

	await screen.findByText('hello')
	expect(tickElement()).not.toBeInTheDocument()
})

test('renders no tick on inbound messages', async () => {
	renderThreadOf(message({ direction: 'inbound', status: 'delivered' }))

	await screen.findByText('hello')
	expect(tickElement()).not.toBeInTheDocument()
})

/**
 * Types a reply and submits it through the composer.
 */
async function sendReply() {
	const user = userEvent.setup()
	await user.type(screen.getByRole('textbox', { name: /reply/i }), 'a message')
	await user.click(screen.getByRole('button', { name: /send/i }))
}

test('shows the mapped explanation when a send is rejected with a known code', async () => {
	renderThreadOf(message({ direction: 'inbound' }))
	server.use(
		graphql.mutation('WhatsAppSendMessage', () =>
			HttpResponse.json({
				data: null,
				errors: [
					{
						message: 'Re-engagement message',
						extensions: { code: 'UPSTREAM', metaCode: 131047 },
					},
				],
			}),
		),
	)
	await screen.findByText('hello')

	await sendReply()

	expect(
		await screen.findByText('Outside the 24-hour window. The customer must message first.'),
	).toBeInTheDocument()
})

test('shows the generic line when a send is rejected with an unknown code', async () => {
	renderThreadOf(message({ direction: 'inbound' }))
	server.use(
		graphql.mutation('WhatsAppSendMessage', () =>
			HttpResponse.json({
				data: null,
				errors: [{ message: 'strange', extensions: { code: 'UPSTREAM', metaCode: 999 } }],
			}),
		),
	)
	await screen.findByText('hello')

	await sendReply()

	expect(await screen.findByText('The reply could not be sent.')).toBeInTheDocument()
})

test('shows the generic line when a rejection carries no code', async () => {
	renderThreadOf(message({ direction: 'inbound' }))
	server.use(
		graphql.mutation('WhatsAppSendMessage', () =>
			HttpResponse.json({
				data: null,
				errors: [{ message: 'upstream failure', extensions: { code: 'UPSTREAM' } }],
			}),
		),
	)
	await screen.findByText('hello')

	await sendReply()

	expect(await screen.findByText('The reply could not be sent.')).toBeInTheDocument()
})

test('shows the generic line when the send fails on the network', async () => {
	renderThreadOf(message({ direction: 'inbound' }))
	server.use(graphql.mutation('WhatsAppSendMessage', () => HttpResponse.error()))
	await screen.findByText('hello')

	await sendReply()

	expect(await screen.findByText('The reply could not be sent.')).toBeInTheDocument()
})
