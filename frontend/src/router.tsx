// SPDX-License-Identifier: AGPL-3.0-or-later

import {
	createRootRoute,
	createRoute,
	createRouter,
} from '@tanstack/react-router'
import type { RouterHistory } from '@tanstack/react-router'

import { ContactRoute, ContactsRoute, NewContactRoute } from './contactRoutes'
import { Home } from './Home'
import { Layout } from './Layout'
import { plugins } from './plugins'
import { NewUserRoute, UsersRoute } from './userRoutes'

const rootRoute = createRootRoute({
	component: Layout,
})

const homeRoute = createRoute({
	getParentRoute: () => rootRoute,
	path: '/',
	component: Home,
})

const contactsRoute = createRoute({
	getParentRoute: () => rootRoute,
	path: '/contacts',
	component: ContactsRoute,
})

const newContactRoute = createRoute({
	getParentRoute: () => rootRoute,
	path: '/contacts/new',
	component: NewContactRoute,
})

const contactRoute = createRoute({
	getParentRoute: () => rootRoute,
	path: '/contacts/$contactId',
	component: ContactRoute,
})

const usersRoute = createRoute({
	getParentRoute: () => rootRoute,
	path: '/users',
	component: UsersRoute,
})

const newUserRoute = createRoute({
	getParentRoute: () => rootRoute,
	path: '/users/new',
	component: NewUserRoute,
})

const routeTree = rootRoute.addChildren([
	homeRoute,
	contactsRoute,
	newContactRoute,
	contactRoute,
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
