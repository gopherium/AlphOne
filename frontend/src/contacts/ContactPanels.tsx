// SPDX-License-Identifier: AGPL-3.0-or-later

import type { ContactPanel } from '@alphone/frontend-sdk'
import { Suspense } from 'react'

/**
 * Renders every plugin contributed panel of one contact.
 * @param props - The contact the panels read and the panels to render.
 * @returns The contributed panels.
 */
export function ContactPanels({
	contactId,
	panels,
}: {
	contactId: string
	panels: ContactPanel[]
}) {
	return (
		<>
			{panels.map(({ id, Panel }) => (
				<Suspense key={id} fallback={null}>
					<Panel contactId={contactId} />
				</Suspense>
			))}
		</>
	)
}
