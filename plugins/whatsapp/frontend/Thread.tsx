// SPDX-License-Identifier: AGPL-3.0-or-later

import {
	Button,
	InputControl,
	LoadingRows,
	PageTitle,
	Stack,
	Text,
	VisuallyHidden,
} from '@alphone/frontend-sdk'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { useEffect, useRef, useState } from 'react'

import {
	SendFailedError,
	fetchConversations,
	fetchMessages,
	mediaURL,
	sendMessage,
} from './api'
import type { Message, MessageMedia } from './api'
import { formatDay, formatDayLabel, formatFileSize, formatTime } from './format'
import { useMediaBlob } from './media'
import { copyForFailureCode, copyForFailureDetail } from './status'

const tickGlyphs: Record<string, { glyph: string; label: string; modifier?: string }> = {
	sent: { glyph: '✓', label: 'Message sent' },
	delivered: { glyph: '✓✓', label: 'Message delivered' },
	read: { glyph: '✓✓', label: 'Message read', modifier: 'read' },
	played: { glyph: '✓✓', label: 'Message played', modifier: 'read' },
	failed: { glyph: '!', label: 'Message not delivered', modifier: 'failed' },
}

const mediaContentTypes = new Set([
	'image',
	'sticker',
	'audio',
	'video',
	'document',
])

const decoratedContent: Record<string, (message: Message) => string> = {
	location: (message) => `📍 ${message.content}`,
	contacts: (message) => `👤 ${message.content || 'Contact card'}`,
	reaction: (message) => message.content || 'Reaction removed',
}

const followThresholdPx = 100

/**
 * Groups messages into list items, inserting a labelled day separator whenever
 * the local calendar date changes between consecutive messages.
 * @param messages - The conversation messages, oldest first.
 * @param now - The current moment, anchoring the Today and Yesterday labels.
 * @param conversationId - The conversation the messages belong to.
 * @returns The list items to render inside the message log.
 */
function threadItems(messages: Message[], now: Date, conversationId: string) {
	const items = []
	let previousDay = ''
	for (const message of messages) {
		const day = formatDay(message.sent_at)
		if (day !== previousDay) {
			items.push(
				<li key={`day-${day}`} className="alphone-message-day">
					<time dateTime={day}>{formatDayLabel(message.sent_at, now)}</time>
				</li>,
			)
			previousDay = day
		}
		items.push(
			<MessageBubble
				key={message.id}
				conversationId={conversationId}
				message={message}
			/>,
		)
	}
	return items
}

/**
 * Renders a single chat bubble, aligned by message direction and carrying a
 * screen-reader-only direction label plus the sent time.
 * @returns The message list item.
 */
function MessageBubble({
	conversationId,
	message,
}: {
	conversationId: string
	message: Message
}) {
	return (
		<li className={`alphone-message alphone-message--${message.direction}`}>
			<div className="alphone-message__bubble">
				<VisuallyHidden>
					{message.direction === 'inbound' ? 'Received' : 'Sent'}
				</VisuallyHidden>
				<MessageBody conversationId={conversationId} message={message} />
				<time
					className="alphone-message__time"
					dateTime={message.sent_at.toISOString()}
				>
					{formatTime(message.sent_at)}
				</time>
				<DeliveryStatus message={message} />
				{message.direction === 'outbound' && message.status === 'failed' ? (
					<Text className="alphone-message__failure">
						{copyForFailureDetail(message.status_detail)}
					</Text>
				) : null}
			</div>
		</li>
	)
}

/**
 * Copy for a failed reply attempt, using the Graph code when the backend
 * surfaced one.
 * @param error - The reply mutation error.
 * @returns The operator-facing line under the composer.
 */
function replyErrorCopy(error: unknown): string {
	if (error instanceof SendFailedError && error.code !== null) {
		return copyForFailureCode(error.code) ?? 'The reply could not be sent.'
	}
	return 'The reply could not be sent.'
}

/**
 * Renders the delivery tick for an outbound message, or nothing before the
 * first status arrives.
 * @returns The tick indicator, or null when there is nothing to show.
 */
function DeliveryStatus({ message }: { message: Message }) {
	if (message.direction !== 'outbound' || !message.status) {
		return null
	}
	const tick = tickGlyphs[message.status]
	if (!tick) {
		return null
	}
	const modifier = tick.modifier ? ` alphone-message__ticks--${tick.modifier}` : ''
	return (
		<span className={`alphone-message__ticks${modifier}`}>
			<span aria-hidden="true">{tick.glyph}</span>
			<VisuallyHidden>{tick.label}</VisuallyHidden>
		</span>
	)
}

/**
 * Renders a message's body by content type: plain text, a media element, or
 * a typed placeholder.
 * @returns The bubble body.
 */
function MessageBody({
	conversationId,
	message,
}: {
	conversationId: string
	message: Message
}) {
	if (mediaContentTypes.has(message.content_type)) {
		return <MediaBody conversationId={conversationId} message={message} />
	}
	if (message.content_type === 'text') {
		return <Text className="alphone-message__content">{message.content}</Text>
	}
	const decorate = decoratedContent[message.content_type]
	return (
		<Text className="alphone-message__content">
			{decorate ? decorate(message) : 'Unsupported message.'}
		</Text>
	)
}

/**
 * Renders a media message's attachment by its download state.
 * @returns The attachment body.
 */
function MediaBody({
	conversationId,
	message,
}: {
	conversationId: string
	message: Message
}) {
	const media = message.media
	if (!media || media.status === 'failed') {
		return <UnavailableAttachment media={media} message={message} />
	}
	if (media.status === 'pending') {
		return (
			<Text role="status" className="alphone-message__content">
				Downloading…
			</Text>
		)
	}
	return (
		<ReadyAttachment
			conversationId={conversationId}
			media={media}
			message={message}
		/>
	)
}

/**
 * Renders the fallback for a media message whose asset never arrived, keeping
 * the document chip while the filename is still known.
 * @returns The unavailable attachment body.
 */
function UnavailableAttachment({
	media,
	message,
}: {
	media: MessageMedia | null | undefined
	message: Message
}) {
	if (media && message.content_type === 'document') {
		return <DocumentChip media={media} caption={message.content} />
	}
	return <Text className="alphone-message__content">Attachment unavailable.</Text>
}

/**
 * Renders a downloaded attachment by kind, followed by any caption.
 * @returns The attachment body.
 */
function ReadyAttachment({
	conversationId,
	media,
	message,
}: {
	conversationId: string
	media: MessageMedia
	message: Message
}) {
	const source = mediaURL(conversationId, message.id)
	switch (message.content_type) {
		case 'image':
		case 'sticker':
			return <MediaImage conversationId={conversationId} message={message} />
		case 'audio':
			return <AudioAttachment media={media} source={source} />
		case 'video':
			return <VideoAttachment message={message} source={source} />
		default:
			return <DocumentChip media={media} caption={message.content} href={source} />
	}
}

/**
 * Renders an audio attachment, labelled as a voice note when WhatsApp marked
 * it as one.
 * @returns The audio body.
 */
function AudioAttachment({
	media,
	source,
}: {
	media: MessageMedia
	source: string
}) {
	const label = media.voice ? 'Voice message' : 'Audio message'
	return (
		<div className="alphone-message__media">
			{media.voice ? (
				<Text className="alphone-message__media-label">Voice message</Text>
			) : null}
			<audio controls preload="metadata" src={source} aria-label={label} />
		</div>
	)
}

/**
 * Renders a video attachment, followed by any caption.
 * @returns The video body.
 */
function VideoAttachment({
	message,
	source,
}: {
	message: Message
	source: string
}) {
	return (
		<div className="alphone-message__media">
			<video controls preload="metadata" src={source} aria-label="Video message" />
			{message.content ? (
				<Text className="alphone-message__caption">{message.content}</Text>
			) : null}
		</div>
	)
}

/**
 * Renders an image or sticker attachment from its cached blob, with loading
 * and failure states.
 * @returns The image body.
 */
function MediaImage({
	conversationId,
	message,
}: {
	conversationId: string
	message: Message
}) {
	const blob = useMediaBlob(conversationId, message.id)
	if (blob.isPending) {
		return (
			<Text role="status" className="alphone-message__content">
				Downloading…
			</Text>
		)
	}
	if (blob.isError) {
		return <Text className="alphone-message__content">Attachment unavailable.</Text>
	}
	const sticker = message.content_type === 'sticker'
	return (
		<div className="alphone-message__media">
			<img
				className={
					sticker
						? 'alphone-message__image alphone-message__image--sticker'
						: 'alphone-message__image'
				}
				src={blob.data}
				alt={sticker ? 'Sticker' : 'Photo'}
			/>
			{message.content ? (
				<Text className="alphone-message__caption">{message.content}</Text>
			) : null}
		</div>
	)
}

/**
 * Renders a document attachment as a named chip, linking to the download
 * when one is available.
 * @returns The document body.
 */
function DocumentChip({
	media,
	caption,
	href,
}: {
	media: MessageMedia
	caption: string
	href?: string
}) {
	const name = media.filename ?? 'Document'
	const label =
		media.file_size === null ? name : `${name} (${formatFileSize(media.file_size)})`
	return (
		<div className="alphone-message__media">
			{href ? (
				<a
					className="alphone-message__document"
					href={href}
					download={media.filename ?? undefined}
				>
					{`📄 ${label}`}
				</a>
			) : (
				<Text className="alphone-message__content">{`📄 ${label} (unavailable)`}</Text>
			)}
			{caption ? (
				<Text className="alphone-message__caption">{caption}</Text>
			) : null}
		</div>
	)
}

/**
 * Renders the conversation's header, naming the contact it belongs to.
 * @returns The thread header element.
 */
function ThreadHeader({ conversationId }: { conversationId: string }) {
	const conversations = useQuery({
		queryKey: ['whatsapp', 'conversations'],
		queryFn: fetchConversations,
	})
	const named = conversations.data?.find(
		(conversation) => conversation.id === conversationId,
	)
	return (
		<header className="alphone-thread__header">
			<PageTitle variant="heading-md">{named?.contact_name ?? 'Conversation'}</PageTitle>
		</header>
	)
}

/**
 * Renders a WhatsApp conversation thread under its header.
 * @returns The thread element.
 */
export function Thread({ conversationId }: { conversationId: string }) {
	return (
		<div className="alphone-thread">
			<ThreadHeader conversationId={conversationId} />
			<ThreadBody conversationId={conversationId} />
		</div>
	)
}

/**
 * Renders the chat log and reply composer for a conversation.
 * The always-mounted conversation list owns the live-update stream.
 * @returns The chat log and composer, or a loading or error message.
 */
function ThreadBody({ conversationId }: { conversationId: string }) {
	const logRef = useRef<HTMLDivElement>(null)
	const followRef = useRef(true)
	const messages = useQuery({
		queryKey: ['whatsapp', 'messages', conversationId],
		queryFn: () => fetchMessages(conversationId),
	})
	useEffect(() => {
		const log = logRef.current
		if (log && followRef.current) {
			log.scrollTop = log.scrollHeight
		}
	}, [messages.data])

	if (messages.isPending) {
		return <LoadingRows label="Loading messages…" />
	}
	if (messages.isError) {
		return <Text role="alert">Messages could not be loaded.</Text>
	}
	return (
		<>
			<div
				role="log"
				aria-label="Messages"
				className="alphone-thread__log"
				tabIndex={0}
				ref={logRef}
				onScroll={(event) => {
					const log = event.currentTarget
					followRef.current =
						log.scrollHeight - log.scrollTop - log.clientHeight <
						followThresholdPx
				}}
			>
				{messages.data.length === 0 ? (
					<Text role="status">No messages yet.</Text>
				) : (
					<ul className="alphone-messages">
						{threadItems(messages.data, new Date(), conversationId)}
					</ul>
				)}
			</div>
			<ThreadComposer conversationId={conversationId} />
		</>
	)
}

/**
 * Renders the reply composer for a conversation.
 * @returns The composer form.
 */
function ThreadComposer({ conversationId }: { conversationId: string }) {
	const queryClient = useQueryClient()
	const [draft, setDraft] = useState('')
	const reply = useMutation({
		mutationFn: (content: string) => sendMessage(conversationId, content),
		onSuccess: () => queryClient.invalidateQueries({ queryKey: ['whatsapp'] }),
		onError: (_error, content) => setDraft(content),
	})
	return (
		<form
			className="alphone-composer"
			onSubmit={(event) => {
				event.preventDefault()
				const content = draft.trim()
				setDraft('')
				reply.mutate(content)
			}}
		>
			<Stack direction="column" gap="sm">
				<Stack direction="row" gap="sm" align="center">
					<InputControl
						label="Reply"
						hideLabelFromVision
						className="alphone-composer__input"
						value={draft}
						onChange={(event) => setDraft(event.target.value)}
					/>
					<Button type="submit" disabled={draft.trim() === '' || reply.isPending}>
						Send
					</Button>
				</Stack>
				{reply.isError ? (
					<Text role="alert">{replyErrorCopy(reply.error)}</Text>
				) : null}
			</Stack>
		</form>
	)
}
