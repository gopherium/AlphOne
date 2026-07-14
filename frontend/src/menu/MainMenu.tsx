// SPDX-License-Identifier: AGPL-3.0-or-later

import { Icon, Stack } from '@alphone/frontend-sdk'
import type { NavItem } from '@alphone/frontend-sdk'
import { Link, useRouter } from '@tanstack/react-router'
import type { AnyRoute } from '@tanstack/react-router'

import { plugins } from '../plugins'
import { coreNav } from './coreNav'

const chevronRightSmall = (
	<svg viewBox="0 0 24 24" xmlns="http://www.w3.org/2000/svg" fill="currentColor">
		<path d="M10.86 8.04L14.28 12.03L10.86 16.02L9.72 15.04L12.3 12.03L9.72 9.02L10.86 8.04Z" />
	</svg>
)

/**
 * Reports whether the route or one of its ancestors declares a sidebar
 * section, mirroring how the layout resolves the drill-down screen from
 * the matched route branch.
 * @param route - The route the nav entry points at.
 * @returns Whether navigating there swaps the sidebar to a section screen.
 */
function drillsIntoSection(route: AnyRoute | undefined): boolean {
	for (let current = route; current; current = current.parentRoute) {
		if (current.options.staticData?.Sidebar) {
			return true
		}
	}
	return false
}

/**
 * Renders a single sidebar menu row: an icon, the label, and when the
 * target route declares a sidebar section, a chevron marking the drill-down.
 * @param item - The nav entry the row links to.
 * @returns The menu link for the given nav item.
 */
function MenuItem({ item }: { item: NavItem }) {
	const routesByPath: Record<string, AnyRoute | undefined> =
		useRouter().routesByPath
	const drillsDown = drillsIntoSection(routesByPath[item.to])
	return (
		<Link to={item.to} className="alphone-menu__item">
			<Stack direction="row" align="center" gap="sm">
				<Icon icon={item.icon} size={24} aria-hidden />
				<span className="alphone-menu__label">{item.label}</span>
				{drillsDown ? (
					<Icon
						icon={chevronRightSmall}
						size={24}
						aria-hidden
						className="alphone-menu__chevron"
					/>
				) : null}
			</Stack>
		</Link>
	)
}

/**
 * Renders the top-level navigation menu: the core sections first, then one
 * row per plugin nav entry.
 * @returns The navigation landmark containing the menu rows.
 */
export function MainMenu() {
	return (
		<Stack direction="column" gap="xs">
			{coreNav.map((item) => (
				<MenuItem key={item.to} item={item} />
			))}
			{plugins.flatMap((plugin) =>
				plugin.nav.map((item) => <MenuItem key={item.to} item={item} />),
			)}
		</Stack>
	)
}
