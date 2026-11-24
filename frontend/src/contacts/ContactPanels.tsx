// SPDX-License-Identifier: AGPL-3.0-or-later

import { useHiddenPlugins } from '@alphone/frontend-sdk'
import { Suspense } from 'react'

import type { PluginPanel } from './panels'

/**
 * Renders every plugin contributed panel of one contact, skipping the panels of a plugin that hides its screens.
 * @param props - The contact the panels read and the panels to render.
 * @returns The contributed panels.
 */
export function ContactPanels({
	contactId,
	panels,
}: {
	contactId: string
	panels: readonly PluginPanel[]
}) {
	const hidden = useHiddenPlugins()
	return (
		<>
			{panels
				.filter(({ plugin }) => !hidden.includes(plugin))
				.map(({ id, Panel }) => (
					<Suspense key={id} fallback={null}>
						<Panel contactId={contactId} />
					</Suspense>
				))}
		</>
	)
}
