// SPDX-License-Identifier: AGPL-3.0-or-later

import { can, useSession } from '@alphone/frontend-sdk'
import type { NavItem } from '@alphone/frontend-sdk'
import { useRouter } from '@tanstack/react-router'
import type { AnyRoute } from '@tanstack/react-router'

/**
 * Returns the check telling whether the session holds the capability a nav entry names and the one its route asks for.
 * @returns The check, true when the reader may open the entry.
 */
export function useReachable(): (item: NavItem) => boolean {
	const session = useSession()
	const routesByPath: Record<string, AnyRoute | undefined> = useRouter().routesByPath
	return (item) =>
		[item.capability, routesByPath[item.to]?.options.staticData?.capability].every(
			(asked) => asked === undefined || can(session, asked),
		)
}
