// SPDX-License-Identifier: AGPL-3.0-or-later

import {
	__,
	Button,
	InputControl,
	LoadingRows,
	PageTitle,
	Skeleton,
	Stack,
	Text,
	VisuallyHidden,
	graphExtensions,
	sprintf,
	useGraphMutation,
	useGraphQuery,
	useGraphSubscription,
} from '@alphone/frontend-sdk'
import type { GraphFailure } from '@alphone/frontend-sdk'
import { useEffect, useMemo, useRef, useState } from 'react'

import { formatDay, formatDayLabel, formatFileSize, formatTime } from './format'
import { useMediaBlob } from './media'
import type { ThreadMessageFragment } from './gql/graphql'
import {
	messageReceivedSubscription,
	sendMessageMutation,
	threadQuery,
} from './operations'
import { copyForFailureCode, copyForFailureDetail } from './status'

/** Message is one thread message as the graph selects it. */
type Message = ThreadMessageFragment

/** MessageMedia is one message's attachment as the graph selects it. */
type MessageMedia = NonNullable<Message['media']>

/**
 * Returns the tick glyphs by status, read fresh so the loaded catalogue answers.
 * @returns The glyphs, keyed by delivery status.
 */
function tickGlyphs(): Record<string, { glyph: string; label: string; modifier?: string }> {
	return {
		sent: { glyph: '✓', label: __('Message sent', 'alphone-whatsapp') },
		delivered: { glyph: '✓✓', label: __('Message delivered', 'alphone-whatsapp') },
		read: { glyph: '✓✓', label: __('Message read', 'alphone-whatsapp'), modifier: 'read' },
		played: { glyph: '✓✓', label: __('Message played', 'alphone-whatsapp'), modifier: 'read' },
		failed: { glyph: '!', label: __('Message not delivered', 'alphone-whatsapp'), modifier: 'failed' },
	}
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
	contacts: (message) => `👤 ${message.content || __('Contact card', 'alphone-whatsapp')}`,
	reaction: (message) => message.content || __('Reaction removed', 'alphone-whatsapp'),
}

const followThresholdPx = 100

/**
 * Groups messages into list items, inserting a labelled day separator whenever
 * the local calendar date changes between consecutive messages.
 * @param messages - The conversation messages, oldest first.
 * @param now - The current moment, anchoring the Today and Yesterday labels.
 * @returns The list items to render inside the message log.
 */
function threadItems(messages: Message[], now: Date) {
	const items = []
	let previousDay = ''
	for (const message of messages) {
		const sentAt = new Date(message.sentAt)
		const day = formatDay(sentAt)
		if (day !== previousDay) {
			items.push(
				<li key={`day-${day}`} className="alphone-message-day">
					<time dateTime={day}>{formatDayLabel(sentAt, now)}</time>
				</li>,
			)
			previousDay = day
		}
		items.push(<MessageBubble key={message.id} message={message} />)
	}
	return items
}

/**
 * Renders a single chat bubble, aligned by message direction and carrying a
 * screen-reader-only direction label plus the sent time.
 * @returns The message list item.
 */
function MessageBubble({ message }: { message: Message }) {
	return (
		<li className={`alphone-message alphone-message--${message.direction}`}>
			<div className="alphone-message__bubble">
				<VisuallyHidden>
					{message.direction === 'inbound'
						? __('Received', 'alphone-whatsapp')
						: __('Sent', 'alphone-whatsapp')}
				</VisuallyHidden>
				<MessageBody message={message} />
				<time className="alphone-message__time" dateTime={message.sentAt}>
					{formatTime(new Date(message.sentAt))}
				</time>
				<DeliveryStatus message={message} />
				{message.direction === 'outbound' && message.status === 'failed' ? (
					<Text className="alphone-message__failure">
						{copyForFailureDetail(message.statusDetail)}
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
function replyErrorCopy(error: GraphFailure): string {
	const metaCode = graphExtensions(error).metaCode
	if (typeof metaCode === 'number') {
		return copyForFailureCode(metaCode) ?? __('The reply could not be sent.', 'alphone-whatsapp')
	}
	return __('The reply could not be sent.', 'alphone-whatsapp')
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
	const tick = tickGlyphs()[message.status]
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
function MessageBody({ message }: { message: Message }) {
	if (mediaContentTypes.has(message.contentType)) {
		return <MediaBody message={message} />
	}
	if (message.contentType === 'text') {
		return <Text className="alphone-message__content">{message.content}</Text>
	}
	const decorate = decoratedContent[message.contentType]
	return (
		<Text className="alphone-message__content">
			{decorate ? decorate(message) : __('Unsupported message.', 'alphone-whatsapp')}
		</Text>
	)
}

/**
 * Renders a media message's attachment by its download state.
 * @returns The attachment body.
 */
function MediaBody({ message }: { message: Message }) {
	const media = message.media
	if (!media || media.status === 'failed') {
		return <UnavailableAttachment media={media} message={message} />
	}
	if (media.status === 'pending') {
		return <MediaGhost />
	}
	return <ReadyAttachment media={media} message={message} />
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
	if (media && message.contentType === 'document') {
		return <DocumentChip media={media} caption={message.content} />
	}
	return <Text className="alphone-message__content">{__('Attachment unavailable.', 'alphone-whatsapp')}</Text>
}

/**
 * Renders a downloaded attachment by kind, followed by any caption.
 * @returns The attachment body.
 */
function ReadyAttachment({
	media,
	message,
}: {
	media: MessageMedia
	message: Message
}) {
	const source = media.downloadPath
	switch (message.contentType) {
		case 'image':
		case 'sticker':
			return <MediaImage media={media} message={message} />
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
	const label = media.voice ? __('Voice message', 'alphone-whatsapp') : __('Audio message', 'alphone-whatsapp')
	return (
		<div className="alphone-message__media">
			{media.voice ? (
				<Text className="alphone-message__media-label">{label}</Text>
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
			<video controls preload="metadata" src={source} aria-label={__('Video message', 'alphone-whatsapp')} />
			{message.content ? (
				<Text className="alphone-message__caption">{message.content}</Text>
			) : null}
		</div>
	)
}

/**
 * Renders a ghost shaped like the attachment that is still downloading.
 * @param props - Whether the attachment is a sticker.
 * @returns The ghost body.
 */
function MediaGhost({ sticker = false }: { sticker?: boolean }) {
	const shape = sticker ? ' alphone-message__ghost--sticker' : ''
	return (
		<div className={`alphone-message__ghost${shape}`}>
			<VisuallyHidden role="status">{__('Downloading…', 'alphone-whatsapp')}</VisuallyHidden>
			<Skeleton className="alphone-message__ghost-block" />
		</div>
	)
}

/**
 * Renders an image or sticker attachment from its cached blob, with loading
 * and failure states.
 * @returns The image body.
 */
function MediaImage({
	media,
	message,
}: {
	media: MessageMedia
	message: Message
}) {
	const blob = useMediaBlob(message.id, media.downloadPath)
	if (blob.isPending) {
		return <MediaGhost sticker={message.contentType === 'sticker'} />
	}
	if (blob.isError) {
		return <Text className="alphone-message__content">{__('Attachment unavailable.', 'alphone-whatsapp')}</Text>
	}
	const sticker = message.contentType === 'sticker'
	return (
		<div className="alphone-message__media">
			<img
				className={
					sticker
						? 'alphone-message__image alphone-message__image--sticker'
						: 'alphone-message__image'
				}
				src={blob.data}
				alt={sticker ? __('Sticker', 'alphone-whatsapp') : __('Photo', 'alphone-whatsapp')}
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
	const name = media.filename ?? __('Document', 'alphone-whatsapp')
	const label =
		media.fileSize === null ? name : `${name} (${formatFileSize(media.fileSize)})`
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
				<Text className="alphone-message__content">
					{`📄 ${sprintf(__('%(name)s (unavailable)', 'alphone-whatsapp'), { name: label })}`}
				</Text>
			)}
			{caption ? (
				<Text className="alphone-message__caption">{caption}</Text>
			) : null}
		</div>
	)
}

/**
 * useThread reads the conversation, appending the arrivals its subscription
 * delivers so a new message needs no reload.
 * @param conversationId - The conversation to read.
 * @returns The thread result beside its merged messages.
 */
function useThread(conversationId: string) {
	const [arrivals, setArrivals] = useState<Message[]>([])
	const [thread] = useGraphQuery({ query: threadQuery, variables: { conversationId } })
	useGraphSubscription(
		{ query: messageReceivedSubscription, variables: { conversationId } },
		(_previous, frame) => {
			setArrivals((current) => [...current, frame.whatsAppMessageReceived])
			return frame
		},
	)
	const loaded = thread.data?.whatsAppConversation
	const messages = useMemo(
		() => mergeById(loaded?.messages ?? [], arrivals),
		[loaded, arrivals],
	)
	return { thread, conversation: loaded, messages, append: appendTo(setArrivals) }
}

/**
 * appendTo returns the appender adding one message to the arrivals.
 * @param setArrivals - The arrivals state setter.
 * @returns The appender.
 */
function appendTo(setArrivals: (update: (current: Message[]) => Message[]) => void) {
	return (message: Message) => {
		setArrivals((current) => [...current, message])
	}
}

/**
 * mergeById returns the loaded messages followed by the arrivals they lack.
 * @param loaded - The messages the thread document returned.
 * @param arrivals - The messages delivered since, newest last.
 * @returns The merged messages, oldest first.
 */
function mergeById(loaded: readonly Message[], arrivals: readonly Message[]): Message[] {
	const known = new Set(loaded.map((message) => message.id))
	return [...loaded, ...arrivals.filter((message) => !known.has(message.id))]
}

/**
 * Renders a WhatsApp conversation thread under its header.
 * @returns The thread element.
 */
export function Thread({ conversationId }: { conversationId: string }) {
	const { thread, conversation, messages, append } = useThread(conversationId)
	return (
		<div className="alphone-thread">
			<header className="alphone-thread__header">
				<PageTitle variant="heading-md">
					{conversation?.contact.name ?? __('Conversation', 'alphone-whatsapp')}
				</PageTitle>
			</header>
			<ThreadBody
				conversationId={conversationId}
				error={thread.error !== undefined}
				loaded={conversation !== undefined}
				messages={messages}
				onSent={append}
			/>
		</div>
	)
}

/**
 * Renders the chat log and reply composer for a conversation.
 * The always-mounted conversation list owns the live-update stream.
 * @returns The chat log and composer, or a loading or error message.
 */
function ThreadBody({
	conversationId,
	error,
	loaded,
	messages,
	onSent,
}: {
	conversationId: string
	error: boolean
	loaded: boolean
	messages: Message[]
	onSent: (message: Message) => void
}) {
	const logRef = useRef<HTMLDivElement>(null)
	const followRef = useRef(true)
	useEffect(() => {
		const log = logRef.current
		if (log && followRef.current) {
			log.scrollTop = log.scrollHeight
		}
	}, [messages])

	if (error) {
		return <Text role="alert">{__('Messages could not be loaded.', 'alphone-whatsapp')}</Text>
	}
	if (!loaded) {
		return <LoadingRows label={__('Loading messages…', 'alphone-whatsapp')} />
	}
	return (
		<>
			<div
				role="log"
				aria-label={__('Messages', 'alphone-whatsapp')}
				className="alphone-thread__log godmin-arrival"
				tabIndex={0}
				ref={logRef}
				onScroll={(event) => {
					const log = event.currentTarget
					followRef.current =
						log.scrollHeight - log.scrollTop - log.clientHeight <
						followThresholdPx
				}}
			>
				{messages.length === 0 ? (
					<Text role="status">{__('No messages yet.', 'alphone-whatsapp')}</Text>
				) : (
					<ul className="alphone-messages">{threadItems(messages, new Date())}</ul>
				)}
			</div>
			<ThreadComposer conversationId={conversationId} onSent={onSent} />
		</>
	)
}

/**
 * Renders the reply composer for a conversation.
 * @returns The composer form.
 */
function ThreadComposer({
	conversationId,
	onSent,
}: {
	conversationId: string
	onSent: (message: Message) => void
}) {
	const [draft, setDraft] = useState('')
	const [reply, send] = useGraphMutation(sendMessageMutation)
	return (
		<form
			className="alphone-composer"
			onSubmit={(event) => {
				event.preventDefault()
				const content = draft.trim()
				setDraft('')
				void send({ conversationId, content }).then((result) => {
					if (result.data) {
						onSent(result.data.whatsAppSendMessage)
						return
					}
					setDraft(content)
				})
			}}
		>
			<Stack direction="column" gap="sm">
				<Stack direction="row" gap="sm" align="center">
					<InputControl
						label={__('Reply', 'alphone-whatsapp')}
						hideLabelFromVision
						className="alphone-composer__input"
						value={draft}
						onChange={(event) => setDraft(event.target.value)}
					/>
					<Button
						type="submit"
						disabled={draft.trim() === '' || reply.fetching}
						loading={reply.fetching}
					>
						{__('Send', 'alphone-whatsapp')}
					</Button>
				</Stack>
				{reply.error ? (
					<Text role="alert">{replyErrorCopy(reply.error)}</Text>
				) : null}
			</Stack>
		</form>
	)
}
