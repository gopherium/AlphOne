// SPDX-License-Identifier: AGPL-3.0-or-later

import { server } from '@alphone/frontend-sdk/testing'
import { screen } from '@testing-library/react'
import { use } from 'react'
import { beforeEach, expect, test, vi } from 'vitest'

import { handlers, importID } from '../handlers'
import { ImportScreen } from '../ImportScreen'
import { renderHosted } from './harness'

const pending = new Promise<void>(() => {})

vi.mock('../RowsTable', () => ({
	default: function SuspendingRows() {
		use(pending)
		return null
	},
}))

beforeEach(() => {
	server.use(...handlers)
})

test('ghosts the preview until its table arrives', async () => {
	renderHosted(<ImportScreen importId={importID} />)

	const label = await screen.findByText('Loading the preview…')
	expect(label.closest('.godmin-loading-rows')).not.toBeNull()
})
