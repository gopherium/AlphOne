// SPDX-License-Identifier: AGPL-3.0-or-later

import { _x, envelope } from '@alphone/frontend-sdk'
import type { MenuItem } from '@alphone/frontend-sdk'

/** nav holds the Email row of the menu, its label read fresh from the loaded catalogue. */
export const nav: MenuItem[] = [{
	get label() {
		return _x('Email', 'admin section', 'alphone-email')
	},
	to: '/email',
	icon: envelope,
}]
