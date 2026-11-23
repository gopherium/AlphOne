// SPDX-License-Identifier: AGPL-3.0-or-later

import { gql, useQuery } from 'urql'
import type { TypedDocumentNode } from 'urql'

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
 * Reads the ids of the plugins that hide their screens in the caller's workspace once somebody is signed in.
 * @returns The hidden plugin ids once served, none while the read loads or after it failed.
 */
export function useHiddenPlugins(): readonly string[] {
	const session = useSession()
	const [result] = useQuery({ query: hiddenPluginsQuery, pause: session === null })
	return result.data?.hiddenPlugins ?? none
}
