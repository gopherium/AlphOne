// SPDX-License-Identifier: AGPL-3.0-or-later

import { Badge, Button, Stack, Text } from '@alphone/frontend-sdk'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { useState } from 'react'

import { fetchMessages, sendMessage } from './api'
import { useLiveUpdates } from './live'

export function Thread({ conversationId }: { conversationId: string }) {
	useLiveUpdates()
	const queryClient = useQueryClient()
	const [draft, setDraft] = useState('')
	const messages = useQuery({
		queryKey: ['whatsapp', 'messages', conversationId],
		queryFn: () => fetchMessages(conversationId),
	})
	const reply = useMutation({
		mutationFn: () => sendMessage(conversationId, draft.trim()),
		onSuccess: () => {
			setDraft('')
			return queryClient.invalidateQueries({
				queryKey: ['whatsapp', 'messages', conversationId],
			})
		},
	})

	if (messages.isPending) {
		return <Text>Loading messages…</Text>
	}
	if (messages.isError) {
		return <Text>Messages could not be loaded.</Text>
	}
	return (
		<Stack direction="column" gap="md">
			{messages.data.length === 0 ? (
				<Text>No messages yet.</Text>
			) : (
				<ul>
					{messages.data.map((message) => (
						<li key={message.id}>
							<Text>{message.content}</Text>
							<Badge>{message.direction}</Badge>
						</li>
					))}
				</ul>
			)}
			<form
				onSubmit={(event) => {
					event.preventDefault()
					reply.mutate()
				}}
			>
				<Stack direction="row" gap="sm" align="center">
					<input
						aria-label="Reply"
						value={draft}
						onChange={(event) => setDraft(event.target.value)}
					/>
					<Button type="submit" disabled={draft.trim() === '' || reply.isPending}>
						Send
					</Button>
				</Stack>
				{reply.isError ? <Text>The reply could not be sent.</Text> : null}
			</form>
		</Stack>
	)
}
