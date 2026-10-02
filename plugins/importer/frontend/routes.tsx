// SPDX-License-Identifier: AGPL-3.0-or-later

import { listSearch } from '@alphone/frontend-sdk'
import { createRoute, lazyRouteComponent, useParams } from '@tanstack/react-router'
import type { AnyRoute } from '@tanstack/react-router'

import { ImportScreen } from './ImportScreen'

/**
 * Renders the import named in the route path.
 * @returns The import screen.
 */
function ImportRouteScreen() {
	const { importId } = useParams({ from: '/import/$importId' })
	return <ImportScreen importId={importId} />
}

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
		component: ImportRouteScreen,
	})
	return [importsRoute, importRoute]
}
