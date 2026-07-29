// SPDX-License-Identifier: AGPL-3.0-or-later

import type { NavItem } from '@alphone/frontend-sdk'

const tasksIcon = (
	<svg viewBox="0 0 24 24" width="24" height="24" aria-hidden="true">
		<path
			fill="currentColor"
			d="M9 16.2 4.8 12l-1.4 1.4L9 19 21 7l-1.4-1.4zM3 19h2v2H3z"
		/>
	</svg>
)

export const tasksNavItem: NavItem = {
	label: 'Tasks',
	to: '/tasks',
	icon: tasksIcon,
}
