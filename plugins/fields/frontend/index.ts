// SPDX-License-Identifier: AGPL-3.0-or-later

import type { FrontendPlugin } from '@alphone/frontend-sdk'

import { ContactFieldsPanel } from './ContactFieldsPanel'
import { fieldsIcon } from './icon'
import { routes } from './routes'

export const plugin: FrontendPlugin = {
	id: 'fields',
	routes,
	nav: [{ label: 'Fields', to: '/fields', icon: fieldsIcon }],
	contactPanels: [{ id: 'fields', Panel: ContactFieldsPanel }],
}
