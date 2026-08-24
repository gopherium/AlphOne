// SPDX-License-Identifier: AGPL-3.0-or-later

import { EmptyState, PageScreen, __, comment } from '@alphone/frontend-sdk'

/**
 * Renders the WhatsApp canvas placeholder shown until a conversation is chosen.
 * @returns The empty-state message for the conversation canvas.
 */
export function Empty() {
	return (
		<PageScreen title={__('Conversations', 'alphone-whatsapp')}>
			<EmptyState.Root className="godmin-empty">
				<EmptyState.Icon icon={comment} />
				<EmptyState.Title>{__('No conversation selected.', 'alphone-whatsapp')}</EmptyState.Title>
				<EmptyState.Description>
					{__('Pick one from the list to read its messages.', 'alphone-whatsapp')}
				</EmptyState.Description>
			</EmptyState.Root>
		</PageScreen>
	)
}
