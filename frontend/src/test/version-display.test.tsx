// SPDX-License-Identifier: AGPL-3.0-or-later

import { screen } from '@testing-library/react'
import { expect, test } from 'vitest'

import { renderAt } from './render'

test('shows the app version in the sidebar', async () => {
	renderAt('/', undefined, '2.5.1')

	expect(await screen.findByText('v2.5.1')).toBeInTheDocument()
})

test('omits the version when it is unavailable', async () => {
	renderAt('/', undefined, null)

	await screen.findByRole('heading', { name: 'AlphOne' })
	expect(screen.queryByText(/^v\d/)).toBeNull()
})
