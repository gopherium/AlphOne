// SPDX-License-Identifier: AGPL-3.0-or-later

import { _x } from '@alphone/frontend-sdk'
import type { NavItem } from '@alphone/frontend-sdk'
import { usersNavItem } from '@gopherium/react-auth/wpds'

import { contactsNavItem } from '../contacts/nav'
import { languageNavItem } from '../i18n/nav'
import { tasksNavItem } from '../tasks/nav'

export const coreNav: NavItem[] = [tasksNavItem, contactsNavItem, usersNavItem, languageNavItem]

/** tokensNavItem leads to the reader's own API tokens. */
const tokensNavItem: NavItem = {
	get label() {
		return _x('API tokens', 'admin section', 'alphone')
	},
	to: '/users/tokens',
	icon: usersNavItem.icon,
}

/** shownInstead names the entry the menu shows in place of a core entry the reader cannot open. */
export const shownInstead = new Map<NavItem, NavItem>([[usersNavItem, tokensNavItem]])
