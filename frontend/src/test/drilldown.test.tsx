// SPDX-License-Identifier: AGPL-3.0-or-later

import { setViewport } from '@alphone/frontend-sdk/testing'
import { screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { expect, test, vi } from 'vitest'

import { renderAt, renderRoutedAt } from './render'

vi.mock('../plugins', async () => ({ plugins: [(await import('./sectionPlugin')).sectionPlugin] }))

/** The class of the wrapper a sidebar screen slides in. */
const WRAPPER = 'godmin-navigation__screen-wrapper'

/**
 * Returns the sidebar region.
 * @returns The region named Navigation.
 */
function sidebar(): Promise<HTMLElement> {
	return screen.findByRole('region', { name: 'Navigation' })
}

/**
 * Opens the section sub menu from its row.
 * @param where - Where the row sits, the sidebar when absent.
 */
async function openSection(where?: HTMLElement) {
	const holder = where ?? (await sidebar())
	await userEvent.click(await within(holder).findByRole('link', { name: 'Section' }))
}

test('swaps the sidebar for the section screen on a section route', async () => {
	renderAt('/section')

	const shown = await sidebar()
	expect(await within(shown).findByText('section sidebar')).toBeInTheDocument()
	expect(screen.getByText('section canvas')).toBeInTheDocument()
	expect(within(shown).queryByRole('link', { name: 'Users' })).toBeNull()
})

test('shows the main menu, not a section screen, at the day of tasks', async () => {
	renderAt('/tasks')

	const shown = await sidebar()
	expect(await within(shown).findByRole('link', { name: 'Section' })).toBeInTheDocument()
	expect(within(shown).queryByText('section sidebar')).toBeNull()
})

test('slides the section in from its row and puts focus on Back', async () => {
	renderAt('/tasks')

	await openSection()

	const back = await within(await sidebar()).findByRole('link', { name: 'Back' })
	expect(within(await sidebar()).getByRole('heading', { level: 1, name: 'Section' })).toBeInTheDocument()
	await waitFor(() => expect(back).toHaveFocus())
	expect(back.closest(`.${WRAPPER}`)).toHaveClass(`${WRAPPER}--forward`)
})

test('slides the main menu back in with focus on the section row', async () => {
	const { router } = renderRoutedAt('/tasks')
	await openSection()

	await userEvent.click(await within(await sidebar()).findByRole('link', { name: 'Back' }))

	const row = await within(await sidebar()).findByRole('link', { name: 'Section' })
	await waitFor(() => expect(row).toHaveFocus())
	expect(row.closest(`.${WRAPPER}`)).toHaveClass(`${WRAPPER}--backward`)
	expect(router.state.location.pathname).toBe('/tasks')
})

test('sends Back on a section opened at its own address to the day of tasks', async () => {
	const { router } = renderRoutedAt('/section')

	await userEvent.click(await within(await sidebar()).findByRole('link', { name: 'Back' }))

	await waitFor(() => expect(router.state.location.pathname).toBe('/tasks'))
	expect(await screen.findByRole('heading', { name: 'Tasks' })).toBeInTheDocument()
})

test('keeps the drawer open on a phone going into the section and coming back', async () => {
	setViewport({ matches: true })
	const { router } = renderRoutedAt('/tasks')
	await userEvent.click(await screen.findByRole('button', { name: 'Open navigation panel' }))
	const drawer = await screen.findByRole('dialog')

	await openSection(drawer)
	await within(drawer).findByRole('heading', { level: 1, name: 'Section' })
	await userEvent.click(within(drawer).getByRole('link', { name: 'Back' }))

	expect(await within(drawer).findByRole('link', { name: 'Section' })).toBeInTheDocument()
	await waitFor(() => expect(router.state.location.pathname).toBe('/tasks'))
	expect(screen.getByRole('dialog')).toBe(drawer)
})
