// SPDX-License-Identifier: AGPL-3.0-or-later

import { SidebarNavigationScreen } from '@alphone/frontend-sdk'

import { ConversationList } from './ConversationList'

/**
 * Renders the WhatsApp sidebar screen: the conversation list under a titled,
 * back-navigable drill-down header.
 * @returns The WhatsApp sidebar navigation screen.
 */
export function WhatsAppSidebar() {
	return (
		<SidebarNavigationScreen title="WhatsApp" backTo="/">
			<ConversationList />
		</SidebarNavigationScreen>
	)
}
