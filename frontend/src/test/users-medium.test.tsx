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

test('draws each bulk action with an icon, the only part of it a medium screen shows', async () => {
	matchingOnly('782px')
	renderAt('/users')

	await userEvent.click((await rowOf('Ada Lovelace')).getByRole('checkbox'))
	await userEvent.click((await rowOf('Ana Lopez')).getByRole('checkbox'))

	const footer = await waitFor(() => {
		const found = document.querySelector<HTMLElement>('.dataviews-bulk-actions-footer__action-buttons')
		expect(found).not.toBeNull()
		return found as HTMLElement
	})
	const bar = within(footer)
	expect((await bar.findByRole('button', { name: 'Disable' })).querySelector('svg')).not.toBeNull()
	expect(bar.getByRole('button', { name: 'Enable' }).querySelector('svg')).not.toBeNull()
})
