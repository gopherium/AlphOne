// SPDX-License-Identifier: AGPL-3.0-or-later

import {
	Button,
	InputControl,
	Stack,
	Text,
	VisuallyHidden,
} from '@alphone/frontend-sdk'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { useEffect, useRef, useState } from 'react'

import { fetchMessages, mediaURL, sendMessage } from './api'
import type { Message, MessageMedia } from './api'
import { formatDay, formatDayLabel, formatFileSize, formatTime } from './format'
import { useMediaBlob } from './media'

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
			</div>
		</li>
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
	switch (message.content_type) {
		case 'text':
			return <Text className="alphone-message__content">{message.content}</Text>
		case 'image':
		case 'sticker':
		case 'audio':
		case 'video':
		case 'document':
			return <MediaBody conversationId={conversationId} message={message} />
		case 'location':
			return (
				<Text className="alphone-message__content">
					{`📍 ${message.content}`}
				</Text>
			)
		case 'contacts':
			return (
				<Text className="alphone-message__content">
					{`👤 ${message.content || 'Contact card'}`}
				</Text>
			)
		case 'reaction':
			return (
				<Text className="alphone-message__content">
					{message.content || 'Reaction removed'}
				</Text>
			)
		default:
			return <Text className="alphone-message__content">Unsupported message.</Text>
	}
}

/**
 * Renders a media message's attachment by its download state and kind,
 * followed by any caption.
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
		if (message.content_type === 'document' && media) {
			return <DocumentChip media={media} caption={message.content} />
		}
		return <Text className="alphone-message__content">Attachment unavailable.</Text>
	}
	if (media.status === 'pending') {
		return (
			<Text role="status" className="alphone-message__content">
				Downloading…
			</Text>
		)
	}
	const source = mediaURL(conversationId, message.id)
	switch (message.content_type) {
		case 'image':
		case 'sticker':
			return <MediaImage conversationId={conversationId} message={message} />
		case 'audio':
			return (
				<div className="alphone-message__media">
					{media.voice ? (
						<Text className="alphone-message__media-label">Voice message</Text>
					) : null}
					<audio
						controls
						preload="metadata"
						src={source}
						aria-label={media.voice ? 'Voice message' : 'Audio message'}
					/>
				</div>
			)
		case 'video':
			return (
				<div className="alphone-message__media">
					<video controls preload="metadata" src={source} aria-label="Video message" />
					{message.content ? (
						<Text className="alphone-message__caption">{message.content}</Text>
					) : null}
				</div>
			)
		default:
			return <DocumentChip media={media} caption={message.content} href={source} />
	}
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
 * Renders a WhatsApp conversation thread as a chat log with a reply composer.
 * The always-mounted conversation list owns the live-update stream.
 * @returns The chat log and composer, or a loading or error message.
 */
export function Thread({ conversationId }: { conversationId: string }) {
	const queryClient = useQueryClient()
	const [draft, setDraft] = useState('')
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
	const reply = useMutation({
		mutationFn: (content: string) => sendMessage(conversationId, content),
		onSuccess: () => queryClient.invalidateQueries({ queryKey: ['whatsapp'] }),
		onError: (_error, content) => setDraft(content),
	})

	if (messages.isPending) {
		return <Text role="status">Loading messages…</Text>
	}
	if (messages.isError) {
		return <Text role="alert">Messages could not be loaded.</Text>
	}
	return (
		<div className="alphone-thread">
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
						<Text role="alert">The reply could not be sent.</Text>
					) : null}
				</Stack>
			</form>
		</div>
	)
}
