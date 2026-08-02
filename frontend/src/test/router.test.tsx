// SPDX-License-Identifier: AGPL-3.0-or-later

import { screen, within } from '@testing-library/react'
import { expect, test } from 'vitest'

import { coreNav } from '../menu/coreNav'
import { plugins } from '../plugins'
import { renderAt } from './render'

test('sends the root path to the day of tasks', async () => {
	renderAt('/')

	expect(await screen.findByRole('heading', { name: 'Tasks' })).toBeInTheDocument()
})

test('shows the AlphOne masthead as a link home, not a heading', async () => {
	renderAt('/')

	const brand = await screen.findByRole('link', { name: 'AlphOne' })

	expect(brand).toHaveAttribute('href', '/')
	expect(screen.queryByRole('heading', { name: 'AlphOne' })).not.toBeInTheDocument()
})

test('renders a navigation entry for every core and plugin section', async () => {
	renderAt('/')

	const nav = await screen.findByRole('navigation')
	expect(within(nav).queryAllByRole('link')).toHaveLength(
		coreNav.length + plugins.flatMap((plugin) => plugin.nav).length,
	)
})

test('marks only entries that drill into a sidebar section with a chevron', async () => {
	renderAt('/')

	const nav = await screen.findByRole('navigation')
	const users = within(nav).getByRole('link', { name: 'Users' })
	const whatsapp = within(nav).getByRole('link', { name: 'WhatsApp' })
	expect(users.querySelector('.alphone-menu__chevron')).toBeNull()
	expect(whatsapp.querySelector('.alphone-menu__chevron')).not.toBeNull()
})

test('frames the active route inside the main content region', async () => {
	renderAt('/')

	const main = await screen.findByRole('main')
	expect(within(main).getByRole('heading', { name: 'Tasks' })).toBeInTheDocument()
	expect(within(main).queryByRole('navigation')).toBeNull()
})
