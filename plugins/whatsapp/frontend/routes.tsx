// SPDX-License-Identifier: AGPL-3.0-or-later

import { createRoute } from '@tanstack/react-router'
import type { AnyRoute } from '@tanstack/react-router'

import { Inbox } from './Inbox'
import { ThreadScreen } from './ThreadScreen'

/**
 * Builds the WhatsApp plugin's route tree under the given parent route.
 * @param parent - The parent route the WhatsApp routes are mounted beneath.
 * @returns An array containing the inbox and conversation-thread routes.
 */
export function routes(parent: AnyRoute): AnyRoute[] {
	const inboxRoute = createRoute({
		getParentRoute: () => parent,
		path: '/whatsapp',
		component: Inbox,
	})
	const threadRoute = createRoute({
		getParentRoute: () => parent,
		path: '/whatsapp/conversations/$conversationId',
		component: ThreadScreen,
	})
	return [inboxRoute, threadRoute]
}
