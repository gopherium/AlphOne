// SPDX-License-Identifier: AGPL-3.0-or-later

import { can, useSession } from '@alphone/frontend-sdk'
import type { NavItem } from '@alphone/frontend-sdk'
import { useRouter } from '@tanstack/react-router'

/**
 * Returns the check telling whether the session holds every capability a nav entry and the routes it opens ask for.
 * @returns The check, true when the reader may open the entry.
 */
export function useReachable(): (item: NavItem) => boolean {
	const session = useSession()
	const router = useRouter()
	return (item) => {
		const [opened] = router.getMatchedRoutes(item.to)
		return [item.capability, ...opened.map((route) => route.options.staticData?.capability)].every(
			(asked) => asked === undefined || can(session, asked),
		)
	}
}
