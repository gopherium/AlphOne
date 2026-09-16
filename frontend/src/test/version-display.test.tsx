// SPDX-License-Identifier: AGPL-3.0-or-later

import { screen } from '@testing-library/react'
import { resetLocaleData, setLocaleData } from '@wordpress/i18n'
import { afterEach, expect, test } from 'vitest'

import { renderAt } from './render'

afterEach(() => {
	resetLocaleData(undefined, 'alphone')
})

test('shapes the version line the way the loaded catalogue says', async () => {
	setLocaleData({ 'v%(version)s': ['version %(version)s'] }, 'alphone')
	renderAt('/', undefined, '1.2.3')

	expect(await screen.findByText('version 1.2.3')).toBeInTheDocument()
})

test('shows the app version in the sidebar', async () => {
	renderAt('/', undefined, '2.5.1')

	expect(await screen.findByText('v2.5.1')).toBeInTheDocument()
})

test('omits the version when it is unavailable', async () => {
	renderAt('/', undefined, null)

	await screen.findByRole('link', { name: 'AlphOne' })
	expect(screen.queryByText(/^v\d/)).toBeNull()
})
