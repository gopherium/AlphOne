// SPDX-License-Identifier: AGPL-3.0-or-later

import type { FrontendPlugin } from '@alphone/frontend-sdk'

import { routes } from './routes'

export const whatsapp: FrontendPlugin = {
	id: 'whatsapp',
	routes,
	nav: [{ label: 'WhatsApp', to: '/whatsapp' }],
}
