// SPDX-License-Identifier: AGPL-3.0-or-later

import { can, useSession } from '@alphone/frontend-sdk'
import type { Capability, NavItem } from '@alphone/frontend-sdk'
import { useRouter } from '@tanstack/react-router'
import type { AnyRoute } from '@tanstack/react-router'

/**
 * Returns the capability a route and each route above it ask for.
 * @param route - The route a nav entry points at.
 * @returns One entry per route, undefined where a route asks for none.
 */
function capabilitiesAskedBy(route: AnyRoute | undefined): (Capability | undefined)[] {
	const asked: (Capability | undefined)[] = []
	for (let current = route; current !== undefined; current = current.parentRoute as AnyRoute | undefined) {
		asked.push(current.options.staticData?.capability)
	}
	return asked
}

/**
 * Returns the check telling whether the session holds every capability a nav entry and its routes ask for.
 * @returns The check, true when the reader may open the entry.
 */
export function useReachable(): (item: NavItem) => boolean {
	const session = useSession()
	const routesByPath: Record<string, AnyRoute | undefined> = useRouter().routesByPath
	return (item) =>
		[item.capability, ...capabilitiesAskedBy(routesByPath[item.to])].every(
			(asked) => asked === undefined || can(session, asked),
		)
}
