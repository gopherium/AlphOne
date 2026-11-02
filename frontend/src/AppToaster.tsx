// SPDX-License-Identifier: AGPL-3.0-or-later

import { Toaster, __, useAdminSettings } from '@alphone/frontend-sdk'
import type { ReactNode } from 'react'

/** RegionProps are the tree a toast region wraps and how long each toast stays, the brick defaults when left out. */
interface RegionProps {
	children: ReactNode
	dismissAfter?: number
	actionDismissAfter?: number
}

/**
 * Renders the toast region with its translated dismiss hint around the given tree.
 * @param props - The tree the region wraps, and how long a toast with no button and one with a button stay.
 * @returns The wrapped tree with its region.
 */
function Region({ children, dismissAfter, actionDismissAfter }: RegionProps) {
	return (
		<Toaster
			dismissAfter={dismissAfter}
			actionDismissAfter={actionDismissAfter}
			dismissLabel={__('Dismiss this notice', 'alphone')}
		>
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
	return (
		<Region dismissAfter={settings?.toastMilliseconds} actionDismissAfter={settings?.toastActionMilliseconds}>
			{children}
		</Region>
	)
}

/**
 * Renders the toast region around a public link screen, each toast staying the brick default, reading no session.
 * @param props - The tree the region wraps.
 * @returns The wrapped tree with its region.
 */
export function PublicToaster({ children }: { children: ReactNode }) {
	return <Region>{children}</Region>
}
