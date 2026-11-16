// SPDX-License-Identifier: AGPL-3.0-or-later

import { _x, globCatalogs } from '@alphone/frontend-sdk'
import type { Catalog, FrontendPlugin } from '@alphone/frontend-sdk'

/** DOMAIN is the text domain the plugin's strings answer under. */
export const DOMAIN = 'alphone-email'

/** catalogs holds the catalogues built beside the plugin's sources. */
const catalogs = import.meta.glob<{ default: Catalog }>('./languages/*.json')

export const plugin: FrontendPlugin = {
	id: 'email',
	routes: () => [],
	nav: [],
	tokenAreas: [{
		value: 'email',
		get label() {
			return _x('email', 'token area', 'alphone-email')
		},
	}],
	locale: { domain: DOMAIN, load: globCatalogs(catalogs) },
}
