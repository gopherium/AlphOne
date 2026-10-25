// SPDX-License-Identifier: AGPL-3.0-or-later

import { _x, language } from '@alphone/frontend-sdk'
import type { MenuItem } from '@alphone/frontend-sdk'

export const languageMenuItem: MenuItem = {
	get label() {
		return _x('Language', 'admin section', 'alphone')
	},
	to: '/language',
	icon: language,
}
