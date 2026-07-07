// SPDX-License-Identifier: AGPL-3.0-or-later

import { createRoute } from '@tanstack/react-router'
import type { AnyRoute } from '@tanstack/react-router'

import { Inbox } from './Inbox'
import { ThreadScreen } from './ThreadScreen'

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
