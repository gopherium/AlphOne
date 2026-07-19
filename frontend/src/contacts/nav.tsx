// SPDX-License-Identifier: AGPL-3.0-or-later

import type { NavItem } from '@alphone/frontend-sdk'

const contactsIcon = (
	<svg viewBox="0 0 24 24" width="24" height="24" aria-hidden="true">
		<path
			fill="currentColor"
			d="M12 12a4 4 0 1 0-4-4 4 4 0 0 0 4 4zm0 2c-3.3 0-8 1.7-8 5v2h16v-2c0-3.3-4.7-5-8-5z"
		/>
	</svg>
)

export const contactsNavItem: NavItem = {
	label: 'Contacts',
	to: '/contacts',
	icon: contactsIcon,
}
