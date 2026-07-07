// SPDX-License-Identifier: AGPL-3.0-or-later

import { screen, within } from '@testing-library/react'
import { expect, test } from 'vitest'

import { plugins } from './plugins'
import { renderAt } from './test/render'

test('serves the home screen at the root path', async () => {
	renderAt('/')

	expect(await screen.findByText(/welcome to alphone/i)).toBeInTheDocument()
})

test('shows the AlphOne masthead as a heading', async () => {
	renderAt('/')

	expect(
		await screen.findByRole('heading', { name: 'AlphOne' }),
	).toBeInTheDocument()
})

test('renders a navigation entry for every registered plugin', async () => {
	renderAt('/')

	const nav = await screen.findByRole('navigation')
	expect(within(nav).queryAllByRole('link')).toHaveLength(
		plugins.flatMap((plugin) => plugin.nav).length,
	)
})
