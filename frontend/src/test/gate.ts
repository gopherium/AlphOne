// SPDX-License-Identifier: AGPL-3.0-or-later

/**
 * Returns a gate a handler waits on, and the function opening it.
 * @returns The gate and its opener.
 */
export function gate(): { held: Promise<void>, release: () => void } {
	let release = () => {}
	const held = new Promise<void>((resolve) => {
		release = resolve
	})
	return { held, release }
}
