// SPDX-License-Identifier: AGPL-3.0-or-later

import { server } from '@alphone/frontend-sdk/testing'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { render, screen } from '@testing-library/react'
import { use } from 'react'
import { beforeEach, expect, test, vi } from 'vitest'

import { handlers, importID } from '../handlers'
import { ImportScreen } from '../ImportScreen'

const pending = new Promise<void>(() => {})

vi.mock('../RowsTable', () => ({
	default: () => {
		use(pending)
		return null
	},
}))

beforeEach(() => {
	server.use(...handlers)
})

test('ghosts the preview until its table arrives', async () => {
	const client = new QueryClient({ defaultOptions: { queries: { retry: false } } })
	render(
		<QueryClientProvider client={client}>
			<ImportScreen importId={importID} />
		</QueryClientProvider>,
	)

	const label = await screen.findByText('Loading the preview…')
	expect(label.closest('.godmin-loading-rows')).not.toBeNull()
})
