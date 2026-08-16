// SPDX-License-Identifier: AGPL-3.0-or-later

import { conversationID, handlers } from './handlers'

/** The whatsapp leaf routes the outline sweep renders, bound to served fixtures. */
export const paths: Record<string, string> = {
	'/whatsapp': '/whatsapp',
	'/whatsapp/': '/whatsapp',
	'/whatsapp/conversations/$conversationId': `/whatsapp/conversations/${conversationID}`,
}

export { handlers }
