// SPDX-License-Identifier: AGPL-3.0-or-later

import { useParams } from '@tanstack/react-router'

import { Thread } from './Thread'

export function ThreadScreen() {
	const { conversationId } = useParams({
		from: '/whatsapp/conversations/$conversationId',
	})
	return <Thread conversationId={conversationId} />
}
