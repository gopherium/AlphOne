// SPDX-License-Identifier: AGPL-3.0-or-later

import type { NavItem } from '@alphone/frontend-sdk'

const globePath =
	'M12 2a10 10 0 1 0 10 10A10 10 0 0 0 12 2zm7.9 9h-3a15.6 15.6 0 0 0-1.2-5.4A8 8 0 0 1 19.9 11z' +
	'M12 4.1c.9 1.2 1.7 3.4 1.9 6.9h-3.8c.2-3.5 1-5.7 1.9-6.9zM4.1 13h3a15.6 15.6 0 0 0 1.2 5.4A8 8 0 0 1 4.1 13z' +
	'm3-2h-3a8 8 0 0 1 4.2-5.4A15.6 15.6 0 0 0 7.1 11zM12 19.9c-.9-1.2-1.7-3.4-1.9-6.9h3.8c-.2 3.5-1 5.7-1.9 6.9z' +
	'm3.7-1.5a15.6 15.6 0 0 0 1.2-5.4h3a8 8 0 0 1-4.2 5.4z'

const languageIcon = (
	<svg viewBox="0 0 24 24" width="24" height="24" aria-hidden="true">
		<path fill="currentColor" d={globePath} />
	</svg>
)

export const languageNavItem: NavItem = {
	label: 'Language',
	to: '/language',
	icon: languageIcon,
}
