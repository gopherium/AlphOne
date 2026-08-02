// SPDX-License-Identifier: AGPL-3.0-or-later

import { server } from '@alphone/frontend-sdk/testing'
import { screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { afterEach, beforeEach, expect, test } from 'vitest'

import { handlers } from '../../../plugins/whatsapp/frontend/handlers'
import { renderAt } from './render'

const realMatchMedia = window.matchMedia

beforeEach(() => server.use(...handlers))
afterEach(() => {
	window.matchMedia = realMatchMedia
})

/**
 * Reports every media query as matching or not, standing in for a viewport.
 * @param matches - What every query should report.
 */
function stubViewport(matches: boolean) {
	window.matchMedia = ((media: string) => ({
		media,
		matches,
		addEventListener: () => {},
		removeEventListener: () => {},
	})) as unknown as typeof window.matchMedia
}

test('replaces the rail with a menu button on a small viewport', async () => {
	stubViewport(true)
	renderAt('/tasks')

	await screen.findByRole('heading', { name: 'Tasks' })

	expect(screen.getByRole('button', { name: 'Open navigation' })).toBeInTheDocument()
	expect(screen.queryByRole('navigation')).toBeNull()
})

test('opens the navigation in a drawer', async () => {
	stubViewport(true)
	renderAt('/tasks')

	await userEvent.click(await screen.findByRole('button', { name: 'Open navigation' }))

	const drawer = await screen.findByRole('dialog')
	expect(within(drawer).getByRole('link', { name: 'Contacts' })).toBeInTheDocument()
	expect(within(drawer).getByRole('button', { name: 'Log out' })).toBeInTheDocument()
})

test('closes the drawer once a section is chosen', async () => {
	stubViewport(true)
	renderAt('/tasks')

	await userEvent.click(await screen.findByRole('button', { name: 'Open navigation' }))
	const drawer = await screen.findByRole('dialog')
	await userEvent.click(within(drawer).getByRole('link', { name: 'Contacts' }))

	await waitFor(() => expect(screen.queryByRole('dialog')).toBeNull())
	expect(await screen.findByRole('heading', { name: 'Contacts' })).toBeInTheDocument()
})

test('keeps the rail and offers no menu button on a wide viewport', async () => {
	stubViewport(false)
	renderAt('/tasks')

	await screen.findByRole('heading', { name: 'Tasks' })

	expect(screen.getByRole('navigation')).toBeInTheDocument()
	expect(screen.queryByRole('button', { name: 'Open navigation' })).toBeNull()
})
