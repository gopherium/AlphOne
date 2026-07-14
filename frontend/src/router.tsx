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
import { NewUserScreen } from './users/NewUserScreen'
import { UsersScreen } from './users/UsersScreen'

const rootRoute = createRootRoute({
	component: Layout,
})

const homeRoute = createRoute({
	getParentRoute: () => rootRoute,
	path: '/',
	component: Home,
})

const usersRoute = createRoute({
	getParentRoute: () => rootRoute,
	path: '/users',
	component: UsersScreen,
})

const newUserRoute = createRoute({
	getParentRoute: () => rootRoute,
	path: '/users/new',
	component: NewUserScreen,
})

const routeTree = rootRoute.addChildren([
	homeRoute,
	usersRoute,
	newUserRoute,
	...plugins.flatMap((plugin) => plugin.routes(rootRoute)),
])

/**
 * Creates the application router with the assembled route tree.
 * @param history - Optional router history instance for controlling navigation state.
 * @returns The configured TanStack router bound to the route tree.
 */
export function createAppRouter(history?: RouterHistory) {
	return createRouter({ routeTree, history })
}

declare module '@tanstack/react-router' {
	interface Register {
		router: ReturnType<typeof createAppRouter>
	}
}
