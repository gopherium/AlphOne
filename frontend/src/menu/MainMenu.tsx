// SPDX-License-Identifier: AGPL-3.0-or-later

import { Icon, Stack } from '@alphone/frontend-sdk'
import type { NavItem } from '@alphone/frontend-sdk'
import { Link } from '@tanstack/react-router'

import { plugins } from '../plugins'

const chevronRightSmall = (
	<svg viewBox="0 0 24 24" xmlns="http://www.w3.org/2000/svg" fill="currentColor">
		<path d="M10.86 8.04L14.28 12.03L10.86 16.02L9.72 15.04L12.3 12.03L9.72 9.02L10.86 8.04Z" />
	</svg>
)

/**
 * Renders a single sidebar menu row: an icon, the label, and a chevron marking
 * the section it drills into.
 * @returns The menu link for the given nav item.
 */
function MenuItem({ item }: { item: NavItem }) {
	return (
		<Link to={item.to} className="alphone-menu__item">
			<Stack direction="row" align="center" gap="sm">
				<Icon icon={item.icon} size={24} aria-hidden />
				<span className="alphone-menu__label">{item.label}</span>
				<Icon
					icon={chevronRightSmall}
					size={24}
					aria-hidden
					className="alphone-menu__chevron"
				/>
			</Stack>
		</Link>
	)
}

/**
 * Renders the top-level navigation menu, one drill-down row per plugin entry.
 * @returns The navigation landmark containing the menu rows.
 */
export function MainMenu() {
	return (
		<Stack direction="column" gap="xs">
			{plugins.flatMap((plugin) =>
				plugin.nav.map((item) => <MenuItem key={item.to} item={item} />),
			)}
		</Stack>
	)
}
