// SPDX-License-Identifier: AGPL-3.0-or-later

import { listSearch } from '@alphone/frontend-sdk'
import { createRoute, lazyRouteComponent } from '@tanstack/react-router'
import type { AnyRoute } from '@tanstack/react-router'

/**
 * Builds the importer plugin's route tree under the given parent route.
 * @param parent - The parent route the importer routes are mounted beneath.
 * @returns The importer routes.
 */
export function routes(parent: AnyRoute): AnyRoute[] {
	const importsRoute = createRoute({
		getParentRoute: () => parent,
		path: '/import',
		validateSearch: listSearch,
		component: lazyRouteComponent(() => import('./ImportsScreen'), 'ImportsScreen'),
	})
	const importRoute = createRoute({
		getParentRoute: () => parent,
		path: '/import/$importId',
		component: lazyRouteComponent(() => import('./ImportScreen'), 'ImportRouteScreen'),
	})
	return [importsRoute, importRoute]
}
