// SPDX-License-Identifier: AGPL-3.0-or-later

import type { FrontendPlugin } from '@alphone/frontend-sdk'

import { whatsappIcon } from './icon'
import { routes } from './routes'

export const plugin: FrontendPlugin = {
	id: 'whatsapp',
	routes,
	nav: [{ label: 'WhatsApp', to: '/whatsapp', icon: whatsappIcon }],
}
