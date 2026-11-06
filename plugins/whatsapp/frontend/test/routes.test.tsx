// SPDX-License-Identifier: AGPL-3.0-or-later

import { HttpResponse, adminSession, graphql, renderPluginAt, server } from '@alphone/frontend-sdk/testing'
import { createRootRoute, createRouter } from '@tanstack/react-router'
import type { AnyRouter } from '@tanstack/react-router'
import { act, fireEvent, screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { afterEach, beforeEach, expect, test, vi } from 'vitest'

import { handlers } from '../handlers'
import { plugin } from '../index'
import { routes } from '../routes'

beforeEach(() => server.use(...handlers))
afterEach(() => vi.restoreAllMocks())

/** The conversation a reader opens first. */
const firstConversation = '019f4a00-0000-7000-8000-000000000001'

/** The conversation a reader opens next. */
const nextConversation = '019f4a00-0000-7000-8000-000000000002'

/** TOAST reads a text inside a toast. */
const TOAST = { selector: '.godmin-toast' }

/**
 * Returns the sidebar the host draws.
 * @returns The region named Navigation.
 */
function sidebar(): Promise<HTMLElement> {
	return screen.findByRole('region', { name: 'Navigation' })
}

test('loads the thread screen in a chunk of its own, off the first paint', () => {
	const rootRoute = createRootRoute()
	const router = createRouter({ routeTree: rootRoute.addChildren(routes(rootRoute)) })
	const { component } = router.routesById['/whatsapp/conversations/$conversationId'].options

	expect(typeof (component as { preload?: unknown }).preload).toBe('function')
})

test('loads the conversation sidebar on demand, off the first paint', () => {
	const [section] = routes(createRootRoute())
	const sidebar = section.options.staticData?.Sidebar as { preload?: unknown } | undefined

	expect(typeof sidebar?.preload).toBe('function')
})

test('contributes a WhatsApp entry to the host navigation', async () => {
	renderPluginAt(plugin, '/', { session: adminSession })

	expect(
		await within(await sidebar()).findByRole('link', { name: 'WhatsApp' }),
	).toBeInTheDocument()
})

test('navigates from the host navigation to the inbox', async () => {
	const user = userEvent.setup()
	renderPluginAt(plugin, '/', { session: adminSession })

	await user.click(await within(await sidebar()).findByRole('link', { name: 'WhatsApp' }))

	expect(await screen.findByText('John Doe')).toBeInTheDocument()
})

test('drills the host sidebar into the WhatsApp sub menu, under its title and Back', async () => {
	renderPluginAt(plugin, '/whatsapp', { session: adminSession })

	const shown = await sidebar()
	expect(await within(shown).findByRole('heading', { level: 1, name: 'WhatsApp' })).toBeInTheDocument()
	expect(within(shown).getByRole('link', { name: 'Back' })).toBeInTheDocument()
	expect(await within(shown).findByText('John Doe')).toBeInTheDocument()
	expect(within(screen.getByRole('main')).getByText('No conversation selected')).toBeInTheDocument()
})

test('takes Back on the WhatsApp sub menu home, the focus on the WhatsApp row', async () => {
	const user = userEvent.setup()
	renderPluginAt(plugin, '/', { session: adminSession })
	await user.click(await within(await sidebar()).findByRole('link', { name: 'WhatsApp' }))

	await user.click(await within(await sidebar()).findByRole('link', { name: 'Back' }))

	expect(await screen.findByText('Test host home')).toBeInTheDocument()
	const row = within(await sidebar()).getByRole('link', { name: 'WhatsApp' })
	await waitFor(() => expect(row).toHaveFocus())
})

test('offers a titled empty state until a conversation is chosen', async () => {
	renderPluginAt(plugin, '/whatsapp', { session: adminSession })

	expect(await screen.findByText('No conversation selected')).toBeInTheDocument()
	expect(
		screen.getByText('Pick one from the list to read its messages.'),
	).toBeInTheDocument()
	expect(screen.getByRole('heading', { level: 3, name: 'No conversation selected' })).toBeInTheDocument()
})

test('serves the thread for a conversation URL', async () => {
	renderPluginAt(
		plugin,
		'/whatsapp/conversations/019f4a00-0000-7000-8000-000000000001',
		{ session: adminSession },
	)

	expect(
		await screen.findByText('Hi, is the order ready?'),
	).toBeInTheDocument()
})

/** Serves each conversation's thread with one message that names the conversation. */
function threadsByConversation() {
	server.use(
		graphql.query('WhatsAppThread', ({ variables }) =>
			HttpResponse.json({
				data: {
					whatsAppConversation: {
						__typename: 'WhatsAppConversation',
						id: variables.conversationId,
						number: null,
						contact: { __typename: 'Contact', id: `contact-${variables.conversationId}`, name: 'Maria Perez' },
						messages: [
							{
								__typename: 'WhatsAppMessage',
								id: `message-${variables.conversationId}`,
								externalId: `wamid.${variables.conversationId}`,
								direction: 'inbound',
								content: `Message in ${variables.conversationId}`,
								contentType: 'text',
								sentAt: '2026-07-06T09:05:00Z',
								status: null,
								statusDetail: null,
								media: null,
							},
						],
					},
				},
			}),
		),
	)
}

/**
 * Opens a conversation's thread and waits for its message.
 * @returns The router the plugin is mounted in.
 */
async function openFirstConversation(): Promise<AnyRouter> {
	const { router } = renderPluginAt(plugin, `/whatsapp/conversations/${firstConversation}`, {
		session: adminSession,
	})
	await screen.findByText(`Message in ${firstConversation}`)
	return router
}

/**
 * Moves the reader to the next conversation and waits for its message.
 * @param router - The router the plugin is mounted in.
 */
async function openNextConversation(router: AnyRouter) {
	await act(() =>
		router.navigate({ to: '/whatsapp/conversations/$conversationId', params: { conversationId: nextConversation } }),
	)
	await screen.findByText(`Message in ${nextConversation}`)
}

test('opens the next conversation on its newest message after the reader scrolled up', async () => {
	vi.spyOn(Element.prototype, 'scrollHeight', 'get').mockReturnValue(400)
	vi.spyOn(Element.prototype, 'clientHeight', 'get').mockReturnValue(100)
	vi.spyOn(Element.prototype, 'scrollTop', 'get').mockReturnValue(0)
	const scrollTopSet = vi.spyOn(Element.prototype, 'scrollTop', 'set').mockImplementation(() => {})
	threadsByConversation()
	const router = await openFirstConversation()
	fireEvent.scroll(screen.getByRole('log', { name: 'Messages' }))
	scrollTopSet.mockClear()

	await openNextConversation(router)

	expect(scrollTopSet).toHaveBeenLastCalledWith(400)
})

test('drops the half-typed reply when the next conversation opens', async () => {
	const user = userEvent.setup()
	threadsByConversation()
	const router = await openFirstConversation()
	await user.type(screen.getByRole('textbox', { name: /reply/i }), 'Half a reply')

	await openNextConversation(router)

	expect(screen.getByRole('textbox', { name: /reply/i })).toHaveValue('')
})

test('drops the live messages of a conversation when the next one opens', async () => {
	threadsByConversation()
	const { router, emit } = renderPluginAt(plugin, `/whatsapp/conversations/${firstConversation}`, {
		session: adminSession,
	})
	await screen.findByText(`Message in ${firstConversation}`)
	emit({
		whatsAppMessageReceived: {
			__typename: 'WhatsAppMessage',
			id: 'arrival-in-first',
			externalId: 'wamid.arrival',
			direction: 'inbound',
			content: 'Arrived in the first conversation',
			contentType: 'text',
			sentAt: '2026-07-06T09:06:00Z',
			status: null,
			statusDetail: null,
			media: null,
		},
	})
	await screen.findByText('Arrived in the first conversation')

	await openNextConversation(router)

	expect(screen.queryByText('Arrived in the first conversation')).toBeNull()
})

test('shows a reply that fails after the reader moved on in a toast, leaving the next composer empty', async () => {
	const user = userEvent.setup()
	let answer: () => void = () => {}
	threadsByConversation()
	server.use(
		graphql.mutation('WhatsAppSendMessage', async () => {
			await new Promise<void>((resolve) => {
				answer = resolve
			})
			return HttpResponse.json({ data: null, errors: [{ message: 'upstream failure' }] })
		}),
	)
	const router = await openFirstConversation()
	await user.type(screen.getByRole('textbox', { name: /reply/i }), 'Ready at 5pm')
	await user.click(screen.getByRole('button', { name: /send/i }))

	await openNextConversation(router)
	answer()

	expect(await screen.findByText('The reply could not be sent.', TOAST)).toBeInTheDocument()
	expect(screen.getByRole('textbox', { name: /reply/i })).toHaveValue('')
})

test('keeps a reply that lands after the reader moved on out of the next conversation, with no toast', async () => {
	const user = userEvent.setup()
	let answer: () => void = () => {}
	threadsByConversation()
	server.use(
		graphql.mutation('WhatsAppSendMessage', async () => {
			await new Promise<void>((resolve) => {
				answer = resolve
			})
			return HttpResponse.json({
				data: {
					whatsAppSendMessage: {
						__typename: 'WhatsAppMessage',
						id: 'reply-in-first',
						externalId: 'wamid.reply',
						direction: 'outbound',
						content: 'Ready at 5pm',
						contentType: 'text',
						sentAt: '2026-07-06T09:10:00Z',
						status: 'sent',
						statusDetail: null,
						media: null,
					},
				},
			})
		}),
	)
	const router = await openFirstConversation()
	await user.type(screen.getByRole('textbox', { name: /reply/i }), 'Ready at 5pm')
	await user.click(screen.getByRole('button', { name: /send/i }))

	await openNextConversation(router)
	await act(async () => {
		answer()
		await new Promise((resolve) => setTimeout(resolve, 100))
	})

	expect(screen.queryByText('Ready at 5pm')).toBeNull()
	expect(document.querySelector('.godmin-toast')).toBeNull()
})

test('navigates from a conversation in the inbox to its thread', async () => {
	const user = userEvent.setup()
	renderPluginAt(plugin, '/whatsapp', { session: adminSession })

	await user.click(await screen.findByRole('link', { name: /John Doe/ }))

	const log = await screen.findByRole('log', { name: 'Messages' })
	expect(
		within(log).getByText('I can pick it up after 5pm.'),
	).toBeInTheDocument()
})
