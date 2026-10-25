// SPDX-License-Identifier: AGPL-3.0-or-later

import { adminSession } from '@alphone/frontend-sdk/testing'

import { coreNav } from '../menu/coreNav'
import { plugins } from '../plugins'

/** everyMenuItem is every core and plugin row the main menu can show. */
export const everyMenuItem = [...coreNav, ...plugins.flatMap((plugin) => plugin.nav)]

/** fullAccessSession is a signed-in admin also holding every capability a menu row names. */
export const fullAccessSession = {
	...adminSession,
	capabilities: [
		...adminSession.capabilities,
		...everyMenuItem.flatMap((item) => (item.capability === undefined ? [] : [item.capability])),
	],
}
