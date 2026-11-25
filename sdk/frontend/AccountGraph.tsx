// SPDX-License-Identifier: AGPL-3.0-or-later

import { sessionQueryKey } from '@gopherium/react-auth'
import { useQueryClient } from '@tanstack/react-query'
import { useState } from 'react'
import type { ReactNode } from 'react'

import { createGraphClient } from './graph'
import type { GraphKeys } from './graph'
import { GraphProvider } from './GraphProvider'
import { useSession } from './session'

/** AccountGraphProps are what an account graph takes. */
interface AccountGraphProps {
	/** keys are the graph keys the cache files each type without an id by. */
	keys: GraphKeys
	/** children are the screens reading the graph. */
	children: ReactNode
}

/**
 * Serves the tree a graph client of the signed-in account's own, a new one for each account.
 * @param props - The graph keys and the screens reading the graph.
 * @returns The provided tree.
 */
export function AccountGraph({ keys, children }: AccountGraphProps) {
	const session = useSession()
	return (
		<AccountClient key={session?.id} keys={keys}>
			{children}
		</AccountClient>
	)
}

/**
 * Serves the tree one graph client built when it mounts.
 * @param props - The graph keys and the screens reading the graph.
 * @returns The provided tree.
 */
function AccountClient({ keys, children }: AccountGraphProps) {
	const queryClient = useQueryClient()
	const [graph] = useState(() =>
		createGraphClient({ keys, onSessionExpired: () => queryClient.setQueryData(sessionQueryKey, null) }),
	)
	return <GraphProvider graph={graph}>{children}</GraphProvider>
}
