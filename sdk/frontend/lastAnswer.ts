// SPDX-License-Identifier: AGPL-3.0-or-later

import { useState } from 'react'

/**
 * Holds the last answer a read gave, so a later read that fails leaves the page and its form on screen.
 * @param data - What the read holds now.
 * @returns What the read holds now or, when it holds nothing, the last answer it gave.
 */
export function useLastAnswer<T>(data: T | null | undefined): T | undefined {
	const [held, setHeld] = useState<T>()
	if (data && data !== held) {
		setHeld(data)
	}
	return data ?? held
}
