// SPDX-License-Identifier: AGPL-3.0-or-later

import { globCatalogs } from '@alphone/frontend-sdk'
import type { Catalog, FrontendPlugin } from '@alphone/frontend-sdk'

import { importerIcon } from './icon'
import { routes } from './routes'

/** DOMAIN is the text domain the plugin's strings answer under. */
export const DOMAIN = 'alphone-importer'

/** catalogs holds the catalogues built beside the plugin's sources. */
const catalogs = import.meta.glob<{ default: Catalog }>('./languages/*.json')

export const plugin: FrontendPlugin = {
	id: 'importer',
	routes,
	nav: [{ label: 'Import', to: '/import', icon: importerIcon }],
	locale: { domain: DOMAIN, load: globCatalogs(catalogs) },
}
