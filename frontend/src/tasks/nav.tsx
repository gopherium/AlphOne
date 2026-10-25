// SPDX-License-Identifier: AGPL-3.0-or-later

import { _x, calendar } from '@alphone/frontend-sdk'
import type { MenuItem } from '@alphone/frontend-sdk'

export const tasksMenuItem: MenuItem = {
	get label() {
		return _x('Tasks', 'admin section', 'alphone')
	},
	to: '/tasks',
	icon: calendar,
}
