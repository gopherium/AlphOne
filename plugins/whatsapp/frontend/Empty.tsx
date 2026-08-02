// SPDX-License-Identifier: AGPL-3.0-or-later

import { EmptyState, PageScreen, comment } from '@alphone/frontend-sdk'

/**
 * Renders the WhatsApp canvas placeholder shown until a conversation is chosen.
 * @returns The empty-state message for the conversation canvas.
 */
export function Empty() {
	return (
		<PageScreen title="Conversations">
			<EmptyState.Root className="alphone-empty">
				<EmptyState.Icon icon={comment} />
				<EmptyState.Title>No conversation selected.</EmptyState.Title>
				<EmptyState.Description>
					Pick one from the list to read its messages.
				</EmptyState.Description>
			</EmptyState.Root>
		</PageScreen>
	)
}
