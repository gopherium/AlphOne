// SPDX-License-Identifier: AGPL-3.0-or-later

import { LoadingScreen } from '@gopherium/godmin'

/**
 * Renders the ghost the auth gate shows while the session loads.
 * @returns The boot loading element.
 */
export function BootLoading() {
	return <LoadingScreen label="Loading…" />
}
