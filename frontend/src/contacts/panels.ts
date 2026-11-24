// SPDX-License-Identifier: AGPL-3.0-or-later

import type { ContactPanel, FrontendPlugin } from '@alphone/frontend-sdk'

import { plugins } from '../plugins'

/** PluginPanel is a panel a plugin contributes to a contact page, beside the id of that plugin. */
export interface PluginPanel extends ContactPanel {
	/** plugin is the id of the plugin contributing the panel. */
	plugin: string
}

/**
 * Returns every panel the given plugins contribute to a contact page, each beside the id of its plugin.
 * @param contributing - The plugins whose panels a contact page renders.
 * @returns The panels, in plugin order.
 */
export function panelsOf(contributing: readonly FrontendPlugin[]): PluginPanel[] {
	return contributing.flatMap((plugin) => (plugin.contactPanels ?? []).map((panel) => ({ ...panel, plugin: plugin.id })))
}

/** contactPanels lists every panel the plugins contribute to a contact page. */
export const contactPanels: PluginPanel[] = panelsOf(plugins)

/**
 * Starts loading every contributed panel that loads on demand, without waiting for any.
 * @param panels - The panels a contact page renders.
 */
export function preloadContactPanels(panels: ContactPanel[]) {
	for (const { Panel } of panels) {
		void Panel.preload?.()
	}
}
