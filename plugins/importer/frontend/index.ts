// SPDX-License-Identifier: AGPL-3.0-or-later

import { _x, globCatalogs, upload } from '@alphone/frontend-sdk'
import type { Catalog, FrontendPlugin } from '@alphone/frontend-sdk'

import { errorTemplates } from './errorTemplates'
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
		icon: upload,
	}],
	tokenAreas: [{
		value: 'imports',
		get label() {
			return _x('imports', 'token area', 'alphone-importer')
		},
	}],
	graphKeys: {
		ImportAssignment: () => null,
		ImportCommitPayload: () => null,
		ImportContact: (data) => String(data.rowId),
		ImportField: (data) => String(data.name),
		ImportRowReason: () => null,
	},
	locale: { domain: DOMAIN, load: globCatalogs(catalogs) },
	errorTemplates,
}
