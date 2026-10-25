// SPDX-License-Identifier: AGPL-3.0-or-later

import { _x, commentAuthorAvatar } from '@alphone/frontend-sdk'
import type { MenuItem } from '@alphone/frontend-sdk'

export const contactsMenuItem: MenuItem = {
	get label() {
		return _x('Contacts', 'admin section', 'alphone')
	},
	to: '/contacts',
	icon: commentAuthorAvatar,
}
