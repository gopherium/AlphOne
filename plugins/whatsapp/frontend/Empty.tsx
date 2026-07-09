// SPDX-License-Identifier: AGPL-3.0-or-later

import { Text } from '@alphone/frontend-sdk'

/**
 * Renders the WhatsApp canvas placeholder shown until a conversation is chosen.
 * @returns The empty-state message for the conversation canvas.
 */
export function Empty() {
	return <Text>Select a conversation to view its messages.</Text>
}
