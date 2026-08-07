// SPDX-License-Identifier: AGPL-3.0-or-later

import { render, screen } from '@testing-library/react'
import { useClient } from 'urql'
import { expect, test, vi } from 'vitest'

import { GraphProvider, useGraph } from '../GraphProvider'
import { createGraphClient } from '../graph'

/** Reports whether the urql client and the graph client agree. */
function Probe() {
	const graph = useGraph()
	const client = useClient()
	return <p>{graph.client === client ? 'same client' : 'different clients'}</p>
}

test('serves the graph client to both urql and the doorbell consumers', () => {
	const graph = createGraphClient({ onSessionExpired: vi.fn() })

	render(
		<GraphProvider graph={graph}>
			<Probe />
		</GraphProvider>,
	)

	expect(screen.getByText('same client')).toBeInTheDocument()
})

test('refuses to serve a graph client outside its provider', () => {
	function Orphan() {
		useGraph()
		return null
	}
	const consoleError = vi.spyOn(console, 'error').mockImplementation(() => {})

	expect(() => render(<Orphan />)).toThrow(/GraphProvider/)

	consoleError.mockRestore()
})
