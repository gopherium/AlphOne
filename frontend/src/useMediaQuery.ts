// SPDX-License-Identifier: AGPL-3.0-or-later

import { useCallback, useSyncExternalStore } from 'react'

/**
 * Reports whether the viewport currently matches the given media query.
 * @param query - The media query to evaluate.
 * @returns True while the query matches.
 */
export function useMediaQuery(query: string): boolean {
	const subscribe = useCallback(
		(onChange: () => void) => {
			const list = window.matchMedia(query)
			list.addEventListener('change', onChange)
			return () => list.removeEventListener('change', onChange)
		},
		[query],
	)
	return useSyncExternalStore(subscribe, () => window.matchMedia(query).matches)
}
