// SPDX-License-Identifier: AGPL-3.0-or-later

import { useQuery } from '@tanstack/react-query'
import { gql } from 'urql'
import type { Client, TypedDocumentNode } from 'urql'

import { useGraph } from './GraphProvider'
import { readFromNetwork } from './networkRead'
import { useSession } from './session'

/** hiddenPluginsQuery reads the ids of the plugins that hide their screens in the caller's workspace. */
const hiddenPluginsQuery: TypedDocumentNode<{ hiddenPlugins: string[] }, Record<string, never>> = gql`
	query HiddenPlugins {
		hiddenPlugins
	}
`

/** none is the answer naming no plugin. */
const none: readonly string[] = []

/**
 * Reads from the network the ids of the plugins that hide their screens, dropping the read once the signal fires.
 * @param client - The graph client reading them.
 * @param signal - The signal that drops the read.
 * @returns The hidden plugin ids, none when the read failed.
 */
function readHiddenPlugins(client: Client, signal: AbortSignal): Promise<readonly string[]> {
	return readFromNetwork(client, hiddenPluginsQuery, signal).then((answer) => answer.data?.hiddenPlugins ?? none)
}

/**
 * Reads the ids of the plugins that hide their screens in the caller's workspace, once for each signed-in account.
 * @returns The hidden plugin ids once served, none while the read loads or after it failed.
 */
export function useHiddenPlugins(): readonly string[] {
	const session = useSession()
	const { client } = useGraph()
	const read = useQuery({
		queryKey: ['hiddenPlugins', session?.id],
		queryFn: ({ signal }) => readHiddenPlugins(client, signal),
		enabled: session !== null,
		staleTime: Infinity,
	})
	return read.data ?? none
}
