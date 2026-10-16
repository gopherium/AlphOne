// SPDX-License-Identifier: AGPL-3.0-or-later

import { createRoute, lazyRouteComponent } from '@tanstack/react-router'
import type { AnyRoute } from '@tanstack/react-router'

/**
 * Builds the fields plugin's route tree under the given parent route.
 * @param parent - The parent route the fields routes are mounted beneath.
 * @returns The fields routes.
 */
export function routes(parent: AnyRoute): AnyRoute[] {
	return [
		createRoute({
			getParentRoute: () => parent,
			path: '/fields',
			component: lazyRouteComponent(() => import('./FieldsScreen'), 'FieldsScreen'),
		}),
	]
}
