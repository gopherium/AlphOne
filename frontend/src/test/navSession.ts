// SPDX-License-Identifier: AGPL-3.0-or-later

import { adminSession } from '@alphone/frontend-sdk/testing'

import { coreNav } from '../menu/coreNav'
import { plugins } from '../plugins'

/** everyNavEntry is every core and plugin entry the main menu can show. */
export const everyNavEntry = [...coreNav, ...plugins.flatMap((plugin) => plugin.nav)]

/** reachingSession is a signed-in admin also holding every capability a nav entry names. */
export const reachingSession = {
	...adminSession,
	capabilities: [
		...adminSession.capabilities,
		...everyNavEntry.flatMap((item) => (item.capability === undefined ? [] : [item.capability])),
	],
}
