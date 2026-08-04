// SPDX-License-Identifier: AGPL-3.0-or-later

import type { FrontendPlugin } from '@alphone/frontend-sdk'

import { importerIcon } from './icon'
import { routes } from './routes'

export const plugin: FrontendPlugin = {
	id: 'importer',
	routes,
	nav: [{ label: 'Import', to: '/import', icon: importerIcon }],
}
