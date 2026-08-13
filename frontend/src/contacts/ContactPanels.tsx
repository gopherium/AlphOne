// SPDX-License-Identifier: AGPL-3.0-or-later

import type { ContactPanel } from '@alphone/frontend-sdk'

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
				<Panel key={id} contactId={contactId} />
			))}
		</>
	)
}
