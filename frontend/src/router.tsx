// SPDX-License-Identifier: AGPL-3.0-or-later

import {
	createRootRoute,
	createRoute,
	createRouter,
} from '@tanstack/react-router'
import type { RouterHistory } from '@tanstack/react-router'

import { Layout } from './Layout'
import { Inbox } from './inbox/Inbox'
import { ThreadScreen } from './thread/ThreadScreen'

const rootRoute = createRootRoute({
	component: Layout,
})

const inboxRoute = createRoute({
	getParentRoute: () => rootRoute,
	path: '/',
	component: Inbox,
})

const threadRoute = createRoute({
	getParentRoute: () => rootRoute,
	path: '/conversations/$conversationId',
	component: ThreadScreen,
})

const routeTree = rootRoute.addChildren([inboxRoute, threadRoute])

export function createAppRouter(history?: RouterHistory) {
	return createRouter({ routeTree, history })
}

declare module '@tanstack/react-router' {
	interface Register {
		router: ReturnType<typeof createAppRouter>
	}
}
