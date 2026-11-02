// SPDX-License-Identifier: AGPL-3.0-or-later

import {
	Badge,
	LoadingScreen,
	NavigationItem,
	NavigationList,
	SelectControl,
	Text,
	VisuallyHidden,
	__,
	comment,
	useGraphQuery,
} from '@alphone/frontend-sdk'
import { useState } from 'react'

import { formatListTime } from './format'
import type { WhatsAppConversationsQuery } from './gql/graphql'
import { useLiveUpdates } from './live'
import { useConnectedNumbers } from './numbers'
import type { ConnectedNumber } from './numbers'
import { conversationsQuery } from './operations'

/** Conversation is one row of the list as the graph selects it. */
type Conversation = WhatsAppConversationsQuery['whatsAppConversations'][number]

/** chosenNumberKey names where the browser tab keeps the number the list is narrowed to. */
const chosenNumberKey = 'alphone-whatsapp-chosen-number'

/** everyNumber is the value of the filter's entry for every number. */
const everyNumber = 'every-number'

/**
 * useChosenNumber holds the number the list is narrowed to in the tab.
 * @param offered - The numbers the filter offers, none when it shows no filter.
 * @returns The chosen number, absent for every number, beside the function choosing another.
 */
function useChosenNumber(offered: readonly ConnectedNumber[]) {
	const [held, setHeld] = useState(() => sessionStorage.getItem(chosenNumberKey))
	return {
		chosen: offered.find((number) => number.identifier === held)?.identifier,
		choose: (next: string | null) => {
			setHeld(next)
			if (next === null) {
				sessionStorage.removeItem(chosenNumberKey)
				return
			}
			sessionStorage.setItem(chosenNumberKey, next)
		},
	}
}

/**
 * Renders the filter choosing which number's conversations the list shows.
 * @returns The number select.
 */
function NumberFilter({
	numbers,
	chosen,
	onChoose,
}: {
	numbers: readonly ConnectedNumber[]
	chosen: string | undefined
	onChoose: (number: string | null) => void
}) {
	const items = [
		{ value: everyNumber, label: __('All numbers', 'alphone-whatsapp') },
		...numbers.map((number) => ({ value: number.identifier, label: number.label || number.identifier })),
	]
	return (
		<SelectControl
			className="alphone-conversation-filter"
			label={__('Filter by number', 'alphone-whatsapp')}
			items={items}
			value={items.find((item) => item.value === (chosen ?? everyNumber))}
			onValueChange={(item) => onChoose(
				numbers.find((number) => number.identifier === item?.value)?.identifier ?? null,
			)}
		/>
	)
}

/**
 * Renders what a row shows of one conversation.
 * @returns The conversation summary.
 */
function ConversationSummary({
	conversation,
	showNumber,
}: {
	conversation: Conversation
	showNumber: boolean
}) {
	return (
		<span className="alphone-conversation">
			<span className="alphone-conversation__top">
				<span className="alphone-conversation__name">
					{conversation.contact.name}
				</span>
				<time
					className="alphone-conversation__time"
					dateTime={conversation.lastActivityAt}
				>
					{formatListTime(new Date(conversation.lastActivityAt), new Date())}
				</time>
			</span>
			{showNumber && conversation.number ? (
				<span className="alphone-conversation__number">
					<VisuallyHidden render={<span />}>
						{__('WhatsApp number', 'alphone-whatsapp')}
					</VisuallyHidden>
					{conversation.number}
				</span>
			) : null}
			<span className="alphone-conversation__bottom">
				<span className="alphone-conversation__preview">
					{conversation.lastMessagePreview ?? ''}
				</span>
				<VisuallyHidden render={<span />}>
					{__('status', 'alphone-whatsapp')}
				</VisuallyHidden>
				<Badge>{conversation.status}</Badge>
			</span>
		</span>
	)
}

/**
 * Renders the conversation rows, or a status message while loading, on error, or when empty.
 * @returns The rows or the status message.
 */
function ConversationRows({
	error,
	rows,
	chosen,
	showNumber,
}: {
	error: boolean
	rows: readonly Conversation[] | undefined
	chosen: string | undefined
	showNumber: boolean
}) {
	if (error) {
		return <Text role="alert">{__('Conversations could not be loaded.', 'alphone-whatsapp')}</Text>
	}
	if (!rows) {
		return <LoadingScreen label={__('Loading conversations…', 'alphone-whatsapp')} />
	}
	if (rows.length === 0) {
		return (
			<Text role="status">
				{chosen === undefined
					? __('No conversations yet.', 'alphone-whatsapp')
					: __('No conversations on this number.', 'alphone-whatsapp')}
			</Text>
		)
	}
	return (
		<NavigationList>
			{rows.map((conversation) => (
				<NavigationItem
					key={conversation.id}
					to="/whatsapp/conversations/$conversationId"
					params={{ conversationId: conversation.id }}
					icon={comment}
				>
					<ConversationSummary conversation={conversation} showNumber={showNumber} />
				</NavigationItem>
			))}
		</NavigationList>
	)
}

/**
 * Renders the sidebar conversation list, with live updates and a filter when the workspace holds more than one number.
 * @returns The filter beside the conversation rows or their status message.
 */
export function ConversationList() {
	useLiveUpdates()
	const numbers = useConnectedNumbers()
	const manyNumbers = numbers.length > 1
	const { chosen, choose } = useChosenNumber(manyNumbers ? numbers : [])
	const [conversations] = useGraphQuery({
		query: conversationsQuery,
		variables: { number: chosen },
		requestPolicy: 'cache-and-network',
	})
	return (
		<>
			{manyNumbers ? <NumberFilter numbers={numbers} chosen={chosen} onChoose={choose} /> : null}
			<ConversationRows
				error={conversations.error !== undefined}
				rows={conversations.data?.whatsAppConversations}
				chosen={chosen}
				showNumber={manyNumbers}
			/>
		</>
	)
}
