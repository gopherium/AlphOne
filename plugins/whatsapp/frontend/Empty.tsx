// SPDX-License-Identifier: AGPL-3.0-or-later

import { ListEmpty, PageScreen, __, comment } from '@alphone/frontend-sdk'

/**
 * Renders the WhatsApp canvas placeholder shown until a conversation is chosen.
 * @returns The empty-state message for the conversation canvas.
 */
export function Empty() {
	return (
		<PageScreen title={__('Conversations', 'alphone-whatsapp')} headingLevel={2}>
			<ListEmpty
				icon={comment}
				title={__('No conversation selected.', 'alphone-whatsapp')}
				hint={__('Pick one from the list to read its messages.', 'alphone-whatsapp')}
			/>
		</PageScreen>
	)
}
