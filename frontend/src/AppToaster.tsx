// SPDX-License-Identifier: AGPL-3.0-or-later

import { Toaster, __, useAdminSettings } from '@alphone/frontend-sdk'
import type { ReactNode } from 'react'

/**
 * Renders the toast region with its translated dismiss control around the given tree.
 * @param props - The tree the region wraps, and how long a toast stays, the brick default when left out.
 * @returns The wrapped tree with its region.
 */
function Region({ children, dismissAfter }: { children: ReactNode, dismissAfter?: number }) {
	return (
		<Toaster dismissAfter={dismissAfter} dismissLabel={__('Dismiss', 'alphone')}>
			{children}
		</Toaster>
	)
}

/**
 * Renders the toast region around the signed-in tree, each toast staying as long as the admin settings say.
 * @param props - The tree the region wraps.
 * @returns The wrapped tree with its region.
 */
export function AppToaster({ children }: { children: ReactNode }) {
	const { settings } = useAdminSettings()
	return <Region dismissAfter={settings?.toastMilliseconds}>{children}</Region>
}

/**
 * Renders the toast region around a public link screen, each toast staying the brick default, reading no session.
 * @param props - The tree the region wraps.
 * @returns The wrapped tree with its region.
 */
export function PublicToaster({ children }: { children: ReactNode }) {
	return <Region>{children}</Region>
}
