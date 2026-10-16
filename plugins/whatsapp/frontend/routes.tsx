// SPDX-License-Identifier: AGPL-3.0-or-later

import { createRoute, lazyRouteComponent } from '@tanstack/react-router'
import type { AnyRoute } from '@tanstack/react-router'

import { Empty } from './Empty'

/**
 * Builds the WhatsApp section route, which carries the sidebar, and the routes that fill its canvas.
 * @param parent - The parent route the WhatsApp routes are mounted beneath.
 * @returns An array containing the WhatsApp section route and its children.
 */
export function routes(parent: AnyRoute): AnyRoute[] {
	const Sidebar = lazyRouteComponent(() => import('./WhatsAppSidebar'), 'WhatsAppSidebar')
	const whatsappRoute = createRoute({
		getParentRoute: () => parent,
		path: '/whatsapp',
		staticData: { Sidebar },
		loader: () => Sidebar.preload?.(),
	})
	const emptyRoute = createRoute({
		getParentRoute: () => whatsappRoute,
		path: '/',
		component: Empty,
	})
	const threadRoute = createRoute({
		getParentRoute: () => whatsappRoute,
		path: 'conversations/$conversationId',
		component: lazyRouteComponent(() => import('./ThreadScreen'), 'ThreadScreen'),
		staticData: { canvas: 'bleed' },
	})
	return [whatsappRoute.addChildren([emptyRoute, threadRoute])]
}
