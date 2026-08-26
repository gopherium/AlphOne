// SPDX-License-Identifier: AGPL-3.0-or-later

import { __ } from '@alphone/frontend-sdk'

/** DOMAIN is the text domain the plugin's messages answer under. */
const DOMAIN = 'alphone-whatsapp'

/**
 * Returns the message each reason of this plugin stands for, read fresh so the
 * catalogue the reader loaded is the one that answers.
 * @returns The messages, keyed by reason.
 */
export function errorTemplates(): Record<string, string> {
	return {
		message_content_required: __('Write something to send.', DOMAIN),
		conversation_not_found: __('That conversation no longer exists.', DOMAIN),
		upstream_failed: __('WhatsApp did not accept the message.', DOMAIN),
		credentials_missing: __('Connect a WhatsApp number before sending.', DOMAIN),
	}
}
