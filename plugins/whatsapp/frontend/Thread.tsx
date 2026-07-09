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

import { fetchMessages, sendMessage } from './api'
import type { Message } from './api'

const followThresholdPx = 100

/**
 * Formats a message timestamp as a zero-padded 24-hour clock time.
 * @param sentAt - The moment the message was sent.
 * @returns The local time in HH:MM form.
 */
function formatTime(sentAt: Date): string {
	const hours = String(sentAt.getHours()).padStart(2, '0')
	const minutes = String(sentAt.getMinutes()).padStart(2, '0')
	return `${hours}:${minutes}`
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
					{message.direction === 'inbound' ? 'Received' : 'Sent'}
				</VisuallyHidden>
				<Text className="alphone-message__content">{message.content}</Text>
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
		onSuccess: () =>
			queryClient.invalidateQueries({
				queryKey: ['whatsapp', 'messages', conversationId],
			}),
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
						{messages.data.map((message) => (
							<MessageBubble key={message.id} message={message} />
						))}
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
