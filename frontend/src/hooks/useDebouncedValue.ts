// SPDX-License-Identifier: AGPL-3.0-or-later

import { useEffect, useState } from 'react'

/**
 * useDebouncedValue returns the value once it has stayed unchanged for the
 * given delay.
 * @param value - The rapidly changing input value.
 * @param delayMs - How long the value must rest before it is published.
 * @returns The debounced value.
 */
export function useDebouncedValue<T>(value: T, delayMs: number): T {
	const [debounced, setDebounced] = useState(value)
	useEffect(() => {
		const timer = setTimeout(() => setDebounced(value), delayMs)
		return () => clearTimeout(timer)
	}, [value, delayMs])
	return debounced
}
