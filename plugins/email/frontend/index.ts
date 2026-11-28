// SPDX-License-Identifier: AGPL-3.0-or-later

import { __, _x, globCatalogs } from '@alphone/frontend-sdk'
import type { Catalog, FrontendPlugin } from '@alphone/frontend-sdk'

import { nav } from './nav'
import { routes } from './routes'

/** DOMAIN is the text domain the plugin's strings answer under. */
export const DOMAIN = 'alphone-email'

/** catalogs holds the catalogues built beside the plugin's sources. */
const catalogs = import.meta.glob<{ default: Catalog }>('./languages/*.json')

export const plugin: FrontendPlugin = {
	id: 'email',
	routes,
	nav,
	tokenAreas: [{
		value: 'email',
		get label() {
			return _x('email', 'token area', 'alphone-email')
		},
		get writeDescription() {
			return __("Reads mail. With an admin's token, also deletes mail and its files. Sends no mail.", 'alphone-email')
		},
	}],
	graphKeys: {
		EmailThreadPage: () => null,
		EmailMessagePage: () => null,
		EmailAddress: () => null,
		EmailParticipant: () => null,
		EmailBody: () => null,
		EmailUsage: () => null,
		EmailDeletion: () => null,
	},
	locale: { domain: DOMAIN, load: globCatalogs(catalogs) },
}
