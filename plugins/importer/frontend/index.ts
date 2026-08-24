// SPDX-License-Identifier: AGPL-3.0-or-later

import { _x, globCatalogs } from '@alphone/frontend-sdk'
import type { Catalog, FrontendPlugin } from '@alphone/frontend-sdk'

import { errorTemplates } from './errorTemplates'
import { importerIcon } from './icon'
import { routes } from './routes'

/** DOMAIN is the text domain the plugin's strings answer under. */
export const DOMAIN = 'alphone-importer'

/** catalogs holds the catalogues built beside the plugin's sources. */
const catalogs = import.meta.glob<{ default: Catalog }>('./languages/*.json')

export const plugin: FrontendPlugin = {
	id: 'importer',
	routes,
	nav: [{
		get label() {
			return _x('Import', 'admin section', 'alphone-importer')
		},
		to: '/import',
		icon: importerIcon,
	}],
	locale: { domain: DOMAIN, load: globCatalogs(catalogs) },
	errorTemplates,
}
