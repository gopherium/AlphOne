// SPDX-License-Identifier: AGPL-3.0-or-later

import { NavigationScreen } from '@alphone/frontend-sdk'

import { ConversationList } from './ConversationList'

/**
 * Renders the WhatsApp sub menu holding the conversation list.
 * @returns The WhatsApp sidebar screen.
 */
export function WhatsAppSidebar() {
	return (
		<NavigationScreen title="WhatsApp">
			<ConversationList />
		</NavigationScreen>
	)
}
