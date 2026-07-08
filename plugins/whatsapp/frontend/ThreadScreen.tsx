// SPDX-License-Identifier: AGPL-3.0-or-later

import { useParams } from '@tanstack/react-router'

import { Thread } from './Thread'

/**
 * Renders the WhatsApp conversation thread for the route's conversation id.
 * @returns The Thread component bound to the current route's conversation id.
 */
export function ThreadScreen() {
	const { conversationId } = useParams({
		from: '/whatsapp/conversations/$conversationId',
	})
	return <Thread conversationId={conversationId} />
}
