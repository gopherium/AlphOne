// SPDX-License-Identifier: AGPL-3.0-or-later

import { listSearch } from '@alphone/frontend-sdk'
import { createRoute, lazyRouteComponent } from '@tanstack/react-router'
import type { AnyRoute } from '@tanstack/react-router'

/**
 * Builds the email plugin's routes under the given parent route.
 * @param parent - The parent route the email routes are mounted beneath.
 * @returns The email routes.
 */
export function routes(parent: AnyRoute): AnyRoute[] {
	const inboxRoute = createRoute({
		getParentRoute: () => parent,
		path: '/email',
		validateSearch: listSearch,
		component: lazyRouteComponent(() => import('./EmailInboxScreen'), 'EmailInboxScreen'),
	})
	return [inboxRoute]
}
