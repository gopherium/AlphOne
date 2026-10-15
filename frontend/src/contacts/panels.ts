// SPDX-License-Identifier: AGPL-3.0-or-later

import type { ContactPanel } from '@alphone/frontend-sdk'

import { plugins } from '../plugins'

/** contactPanels lists every panel the plugins contribute to a contact page. */
export const contactPanels: ContactPanel[] = plugins.flatMap((plugin) => plugin.contactPanels ?? [])

/**
 * Starts loading every contributed panel that loads on demand, without waiting for any.
 * @param panels - The panels a contact page renders.
 */
export function preloadContactPanels(panels: ContactPanel[]) {
	for (const { Panel } of panels) {
		void Panel.preload?.()
	}
}
