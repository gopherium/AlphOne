// SPDX-License-Identifier: AGPL-3.0-or-later

import { GraphProvider } from '@alphone/frontend-sdk'
import { fakeGraphClient, server } from '@alphone/frontend-sdk/testing'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { act, render, screen, waitFor } from '@testing-library/react'
import { HttpResponse, graphql, http } from 'msw'
import { beforeEach, expect, test, vi } from 'vitest'

import { handlers } from '../handlers'
import { Thread } from '../Thread'

const conversationId = '019f4a00-0000-7000-8000-000000000001'
const messagesPath = '/api/plugins/whatsapp/conversations/:conversationId/messages'
const mediaPath = `${messagesPath}/:messageId/media`

let messageCounter = 0

beforeEach(() => {
	server.use(...handlers)
	messageCounter = 0
	let urlCounter = 0
	URL.createObjectURL = vi.fn(() => {
		urlCounter += 1
		return `blob:mock-${urlCounter}`
	})
})

/**
 * Builds a raw message fixture with unique identifiers.
 * @param overrides - Fields overriding the defaults.
 * @returns The raw message payload.
 */
function message(overrides: Record<string, unknown>): Record<string, unknown> {
	messageCounter += 1
	const id = `mid-${messageCounter}`
	const built: Record<string, unknown> = {
		__typename: 'WhatsAppMessage',
		id,
		externalId: `wamid.media.${messageCounter}`,
		direction: 'inbound',
		content: '',
		contentType: 'text',
		sentAt: '2026-07-06T09:05:00Z',
		status: null,
		statusDetail: null,
		media: null,
		...overrides,
	}
	if (built.media) {
		built.media = { ...(built.media as object), downloadPath: mediaURL(conversationId, id) }
	}
	return built
}

/**
 * Builds a media payload fixture.
 * @param overrides - Fields overriding the stored-image defaults.
 * @returns The media payload.
 */
function media(overrides: Record<string, unknown> = {}): Record<string, unknown> {
	return {
		__typename: 'WhatsAppMedia',
		status: 'stored',
		mimeType: 'image/jpeg',
		filename: null,
		fileSize: null,
		voice: false,
		animated: false,
		...overrides,
	}
}

/**
 * Serves the given raw messages for the thread under test.
 * @param messages - The raw message payloads to return.
 */
function threadOf(...messages: Array<Record<string, unknown>>) {
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
}

/**
 * Builds the download path of a message's stored blob.
 * @param conversation - The conversation owning the message.
 * @param messageId - The message whose blob to address.
 * @returns The media endpoint path.
 */
function mediaURL(conversation: string, messageId: string): string {
	return `/api/plugins/whatsapp/conversations/${conversation}/messages/${messageId}/media`
}

/**
 * Serves media blobs and counts how many downloads were made.
 * @returns The download counter.
 */
function serveMediaBlobs(): { hits: number } {
	const counter = { hits: 0 }
	server.use(
		http.get(mediaPath, () => {
			counter.hits += 1
			return HttpResponse.arrayBuffer(new TextEncoder().encode('img').buffer as ArrayBuffer, {
				headers: { 'Content-Type': 'image/jpeg' },
			})
		}),
	)
	return counter
}

/**
 * Renders the thread under a fresh query client without retries.
 * @returns The query client backing the render.
 */
function renderThread(): QueryClient {
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
	return client
}

test('renders a stored image with its caption', async () => {
	threadOf(message({ contentType: 'image', content: 'the invoice', media: media() }))
	const downloads = serveMediaBlobs()

	renderThread()

	const image = await screen.findByAltText('Photo')
	expect(image).toHaveAttribute('src', 'blob:mock-1')
	expect(image).toHaveClass('alphone-message__image')
	expect(image).not.toHaveClass('alphone-message__image--sticker')
	expect(screen.getByText('the invoice')).toBeInTheDocument()
	expect(downloads.hits).toBe(1)
})

test('renders a sticker smaller and without a caption', async () => {
	threadOf(message({ contentType: 'sticker', media: media({ mimeType: 'image/webp', animated: true }) }))
	serveMediaBlobs()

	renderThread()

	const image = await screen.findByAltText('Sticker')
	expect(image).toHaveClass('alphone-message__image--sticker')
})

test('shows a downloading state without touching the media endpoint', async () => {
	threadOf(message({ contentType: 'image', media: media({ status: 'pending' }) }))
	const downloads = serveMediaBlobs()

	renderThread()

	expect(await screen.findByText('Downloading…')).toBeInTheDocument()
	expect(downloads.hits).toBe(0)
})

test('ghosts the bubble in the shape of the media it is downloading', async () => {
	threadOf(message({ contentType: 'image', media: media({ status: 'pending' }) }))
	serveMediaBlobs()

	renderThread()

	const status = await screen.findByText('Downloading…')
	expect(status).toHaveAttribute('role', 'status')
	const bubble = status.closest('.alphone-message__ghost')
	expect(bubble).not.toBeNull()
	expect(bubble?.querySelector('[aria-hidden="true"]')).not.toBeNull()
})

test('ghosts a sticker at sticker size while its blob arrives', async () => {
	threadOf(message({ contentType: 'sticker', media: media({ status: 'ready' }) }))
	let release: () => void = () => {}
	const held = new Promise<void>((resolve) => {
		release = resolve
	})
	server.use(
		http.get(mediaPath, async () => {
			await held
			return HttpResponse.arrayBuffer(new TextEncoder().encode('img').buffer as ArrayBuffer, {
				headers: { 'Content-Type': 'image/jpeg' },
			})
		}),
	)

	renderThread()

	const status = await screen.findByText('Downloading…')
	expect(status.closest('.alphone-message__ghost--sticker')).not.toBeNull()
	release()
	await screen.findByAltText('Sticker')
})

test('shows failed attachments as unavailable', async () => {
	threadOf(message({ contentType: 'image', media: media({ status: 'failed' }) }))
	const downloads = serveMediaBlobs()

	renderThread()

	expect(await screen.findByText('Attachment unavailable.')).toBeInTheDocument()
	expect(downloads.hits).toBe(0)
})

test('shows media messages without a media record as unavailable', async () => {
	threadOf(message({ contentType: 'video', media: null }))

	renderThread()

	expect(await screen.findByText('Attachment unavailable.')).toBeInTheDocument()
})

test('shows an unavailable state when the blob download fails', async () => {
	threadOf(message({ contentType: 'image', media: media() }))
	server.use(http.get(mediaPath, () => new HttpResponse(null, { status: 500 })))

	renderThread()

	expect(await screen.findByText('Attachment unavailable.')).toBeInTheDocument()
})

test('renders a voice note with a native player', async () => {
	const item = message({ contentType: 'audio', media: media({ mimeType: 'audio/ogg', voice: true }) })
	threadOf(item)

	renderThread()

	expect(await screen.findByText('Voice message')).toBeInTheDocument()
	const audio = document.querySelector('audio')
	expect(audio).toHaveAttribute('src', mediaURL(conversationId, item.id as string))
	expect(audio).toHaveAttribute('aria-label', 'Voice message')
	expect(audio).toHaveAttribute('preload', 'metadata')
})

test('renders an audio file without the voice label', async () => {
	threadOf(message({ contentType: 'audio', media: media({ mimeType: 'audio/mpeg' }) }))

	renderThread()

	await waitFor(() => expect(document.querySelector('audio')).toBeInTheDocument())
	expect(screen.queryByText('Voice message')).not.toBeInTheDocument()
	expect(document.querySelector('audio')).toHaveAttribute('aria-label', 'Audio message')
})

test('renders a video with its caption', async () => {
	const item = message({
		contentType: 'video',
		content: 'look at this',
		media: media({ mimeType: 'video/mp4' }),
	})
	threadOf(item)

	renderThread()

	expect(await screen.findByText('look at this')).toBeInTheDocument()
	const video = document.querySelector('video')
	expect(video).toHaveAttribute('src', mediaURL(conversationId, item.id as string))
})

test('renders a video without a caption', async () => {
	threadOf(message({ contentType: 'video', media: media({ mimeType: 'video/mp4' }) }))

	renderThread()

	await waitFor(() => expect(document.querySelector('video')).toBeInTheDocument())
	expect(document.querySelector('.alphone-message__caption')).not.toBeInTheDocument()
})

test('renders a stored document as a download link', async () => {
	const item = message({
		contentType: 'document',
		media: media({ mimeType: 'application/pdf', filename: 'receipt.pdf', fileSize: 2048 }),
	})
	threadOf(item)

	renderThread()

	const link = await screen.findByRole('link', { name: '📄 receipt.pdf (2 KB)' })
	expect(link).toHaveAttribute('href', mediaURL(conversationId, item.id as string))
	expect(link).toHaveAttribute('download', 'receipt.pdf')
})

test('renders a nameless document without a size', async () => {
	threadOf(message({ contentType: 'document', media: media({ mimeType: 'application/pdf' }) }))

	renderThread()

	expect(await screen.findByRole('link', { name: '📄 Document' })).toBeInTheDocument()
})

test('renders a failed document as a chip without a link', async () => {
	threadOf(
		message({
			contentType: 'document',
			content: 'the contract',
			media: media({
				status: 'failed',
				mimeType: 'application/pdf',
				filename: 'contract.pdf',
				fileSize: 40 << 20,
			}),
		}),
	)

	renderThread()

	expect(await screen.findByText('📄 contract.pdf (40.0 MB) (unavailable)')).toBeInTheDocument()
	expect(screen.queryByRole('link')).not.toBeInTheDocument()
	expect(screen.getByText('the contract')).toBeInTheDocument()
})

test('renders typed placeholders for non-media types', async () => {
	threadOf(
		message({ contentType: 'location', content: 'British Museum' }),
		message({ contentType: 'contacts', content: 'Ana García, Luis Ruiz' }),
		message({ contentType: 'contacts', content: '' }),
		message({ contentType: 'reaction', content: '👍' }),
		message({ contentType: 'reaction', content: '' }),
		message({ contentType: 'poll', content: '' }),
	)

	renderThread()

	expect(await screen.findByText('📍 British Museum')).toBeInTheDocument()
	expect(screen.getByText('👤 Ana García, Luis Ruiz')).toBeInTheDocument()
	expect(screen.getByText('👤 Contact card')).toBeInTheDocument()
	expect(screen.getByText('👍')).toBeInTheDocument()
	expect(screen.getByText('Reaction removed')).toBeInTheDocument()
	expect(screen.getByText('Unsupported message.')).toBeInTheDocument()
})

test('downloads at most two blobs at once', async () => {
	threadOf(
		message({ contentType: 'image', media: media() }),
		message({ contentType: 'image', media: media() }),
		message({ contentType: 'image', media: media() }),
	)
	let active = 0
	let peak = 0
	server.use(
		http.get(mediaPath, async () => {
			active += 1
			peak = Math.max(peak, active)
			await new Promise((resolve) => setTimeout(resolve, 25))
			active -= 1
			return HttpResponse.arrayBuffer(new TextEncoder().encode('img').buffer as ArrayBuffer, {
				headers: { 'Content-Type': 'image/jpeg' },
			})
		}),
	)

	renderThread()

	await waitFor(() => expect(screen.getAllByAltText('Photo')).toHaveLength(3))
	expect(peak).toBe(2)
})

test('live invalidations never re-download blobs', async () => {
	threadOf(message({ contentType: 'image', media: media() }))
	const downloads = serveMediaBlobs()
	const client = renderThread()
	await screen.findByAltText('Photo')
	expect(downloads.hits).toBe(1)

	await act(() => client.invalidateQueries({ queryKey: ['whatsapp'] }))

	await waitFor(() => expect(client.isFetching()).toBe(0))
	expect(screen.getByAltText('Photo')).toBeInTheDocument()
	expect(downloads.hits).toBe(1)
})
