// SPDX-License-Identifier: AGPL-3.0-or-later

import {
	createRootRoute,
	createRoute,
	createRouter,
} from '@tanstack/react-router'
import type { RouterHistory } from '@tanstack/react-router'

import { Home } from './Home'
import { Layout } from './Layout'
import { plugins } from './plugins'

const rootRoute = createRootRoute({
	component: Layout,
})

const homeRoute = createRoute({
	getParentRoute: () => rootRoute,
	path: '/',
	component: Home,
})

const routeTree = rootRoute.addChildren([
	homeRoute,
	...plugins.flatMap((plugin) => plugin.routes(rootRoute)),
])

export function createAppRouter(history?: RouterHistory) {
	return createRouter({ routeTree, history })
}

declare module '@tanstack/react-router' {
	interface Register {
		router: ReturnType<typeof createAppRouter>
	}
}
