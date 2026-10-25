// SPDX-License-Identifier: AGPL-3.0-or-later

import { _x } from '@alphone/frontend-sdk'
import type { MenuItem } from '@alphone/frontend-sdk'
import { usersMenuItem } from '@gopherium/react-auth/wpds'

import { contactsMenuItem } from '../contacts/nav'
import { languageMenuItem } from '../i18n/nav'
import { tasksMenuItem } from '../tasks/nav'

/** tokensMenuItem leads to the reader's own API tokens. */
const tokensMenuItem: MenuItem = {
	get label() {
		return _x('API tokens', 'admin section', 'alphone')
	},
	to: '/users/tokens',
	icon: usersMenuItem.icon,
}

/** usersRow leads to the Users screen, or to the reader's own API tokens for a reader who cannot open it. */
const usersRow: MenuItem = {
	get label() {
		return usersMenuItem.label
	},
	to: usersMenuItem.to,
	icon: usersMenuItem.icon,
	fallback: tokensMenuItem,
}

export const coreNav: MenuItem[] = [tasksMenuItem, contactsMenuItem, usersRow, languageMenuItem]
