// SPDX-License-Identifier: AGPL-3.0-or-later

import { createContext, useContext } from 'react'
import type { ReactNode } from 'react'
import { Provider } from 'urql'

import type { GraphClient } from './graph'

const GraphContext = createContext<GraphClient | undefined>(undefined)

/**
 * GraphProvider serves one graph client to the screens and the doorbell.
 * @param props - The graph client and the tree consuming it.
 * @returns The provided tree.
 */
export function GraphProvider(props: { graph: GraphClient; children: ReactNode }) {
	return (
		<GraphContext.Provider value={props.graph}>
			<Provider value={props.graph.client}>{props.children}</Provider>
		</GraphContext.Provider>
	)
}

/**
 * Returns the graph client the enclosing provider serves.
 * @returns The graph client beside its doorbell.
 */
export function useGraph(): GraphClient {
	const graph = useContext(GraphContext)
	if (!graph) {
		throw new Error('useGraph must be called inside a GraphProvider')
	}
	return graph
}
