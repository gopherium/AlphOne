// SPDX-License-Identifier: AGPL-3.0-or-later

import { NavScreen } from '@gopherium/godmin'
import { Link } from '@tanstack/react-router'
import type { ReactElement, ReactNode } from 'react'

interface SidebarNavigationScreenProps {
	title: string
	backTo?: string
	description?: ReactNode
	actions?: ReactNode
	footer?: ReactNode
	children: ReactNode
}

/**
 * Renders a drill-down sidebar screen, routing its back link to the given path.
 * @param props - The title, the path back, and the screen regions.
 * @returns The sidebar screen element.
 */
export function SidebarNavigationScreen({ backTo, ...regions }: SidebarNavigationScreenProps) {
	const back =
		backTo === undefined ? undefined : (<Link to={backTo} /> as ReactElement<Record<string, unknown>>)
	return <NavScreen back={back} {...regions} />
}
