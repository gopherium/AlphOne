// SPDX-License-Identifier: AGPL-3.0-or-later

import { createRoute } from '@tanstack/react-router'
import type { AnyRoute } from '@tanstack/react-router'

import { Empty } from './Empty'
import { ThreadScreen } from './ThreadScreen'
import { WhatsAppSidebar } from './WhatsAppSidebar'

/**
 * Builds the WhatsApp plugin's route tree under the given parent route. The
 * section route carries the sidebar screen; its children fill the canvas.
 * @param parent - The parent route the WhatsApp routes are mounted beneath.
 * @returns An array containing the WhatsApp section route and its children.
 */
export function routes(parent: AnyRoute): AnyRoute[] {
	const whatsappRoute = createRoute({
		getParentRoute: () => parent,
		path: '/whatsapp',
		staticData: { Sidebar: WhatsAppSidebar },
	})
	const emptyRoute = createRoute({
		getParentRoute: () => whatsappRoute,
		path: '/',
		component: Empty,
	})
	const threadRoute = createRoute({
		getParentRoute: () => whatsappRoute,
		path: 'conversations/$conversationId',
		component: ThreadScreen,
	})
	return [whatsappRoute.addChildren([emptyRoute, threadRoute])]
}
