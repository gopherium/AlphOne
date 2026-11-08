// SPDX-License-Identifier: AGPL-3.0-or-later

import { Link, Stack, can, useSession } from '@alphone/frontend-sdk'
import type { AccountLink } from '@alphone/frontend-sdk'

/**
 * Renders the plugins' links under the account panel that the signed-in account may follow.
 * @param props - The links the plugins declare, in plugin order.
 * @returns The links, or null when none is for this account.
 */
export function AccountLinks({ links }: { links: AccountLink[] }) {
	const session = useSession()
	const shown = links.filter(
		(link) => session !== null && (link.capability === undefined || can(session, link.capability)),
	)
	if (shown.length === 0) {
		return null
	}
	return (
		<Stack direction="column" gap="sm">
			{shown.map((link) => (
				<Link key={link.href} href={link.href} rel="noreferrer" openInNewTab={link.openInNewTab}>
					{link.label}
				</Link>
			))}
		</Stack>
	)
}
