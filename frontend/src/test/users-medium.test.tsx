// SPDX-License-Identifier: AGPL-3.0-or-later

import { HttpResponse, graphql, server } from '@alphone/frontend-sdk/testing'
import { screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { beforeAll, beforeEach, expect, onTestFinished, test, vi } from 'vitest'

import { renderAt } from './render'

/**
 * Makes only the media queries naming the given width match, as on a screen that narrow.
 * @param width - The width a matching query names.
 */
function matchingOnly(width: string) {
	const spy = vi.spyOn(window, 'matchMedia')
	onTestFinished(() => spy.mockRestore())
	spy.mockImplementation(
		(media) =>
			({
				matches: media.includes(width),
				media,
				onchange: null,
				addEventListener: () => {},
				removeEventListener: () => {},
				addListener: () => {},
				removeListener: () => {},
				dispatchEvent: () => false,
			}) as MediaQueryList,
	)
}

/**
 * Returns the table row holding the given text.
 * @param text - A name the row shows.
 * @returns The row queries.
 */
async function rowOf(text: string) {
	return within(await screen.findByRole('row', { name: new RegExp(text) }))
}

beforeAll(async () => {
	await import('../users/UsersScreen')
})

beforeEach(() => {
	server.use(
		graphql.query('Users', () =>
			HttpResponse.json({
				data: {
					users: [
						{
							__typename: 'User',
							id: '0198b2f0-0000-7000-8000-0000000000ff',
							email: 'ada@example.com',
							name: 'Ada Lovelace',
							disabled: false,
							confirmed: true,
							createdAt: '2026-07-07T10:00:00Z',
							role: 'member',
							withinReach: true,
						},
						{
							__typename: 'User',
							id: '0198b2f0-0000-7000-8000-0000000000fe',
							email: 'ana@example.com',
							name: 'Ana Lopez',
							disabled: true,
							confirmed: true,
							createdAt: '2026-07-08T10:00:00Z',
							role: 'member',
							withinReach: true,
						},
					],
				},
			}),
		),
	)
})

test('puts every bulk action in one Actions menu at 782px and below, as DataViews draws a medium screen', async () => {
	matchingOnly('782px')
	renderAt('/users')

	await userEvent.click((await rowOf('Ada Lovelace')).getByRole('checkbox'))
	await userEvent.click((await rowOf('Ana Lopez')).getByRole('checkbox'))

	const actions = await waitFor(() => {
		const found = document.querySelector<HTMLElement>('.dataviews-bulk-actions__actions')
		expect(found).not.toBeNull()
		return found as HTMLElement
	})
	const bar = within(actions)
	const menu = await bar.findByRole('button', { name: 'Actions' })
	expect(bar.getAllByRole('button')).toEqual([menu])
	await userEvent.click(menu)
	const offered = (await screen.findAllByRole('menuitem')).map((item) => item.textContent)
	expect(offered).toEqual(['Change role', 'Enable', 'Disable'])
})
