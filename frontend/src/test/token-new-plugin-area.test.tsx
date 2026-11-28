// SPDX-License-Identifier: AGPL-3.0-or-later

import { HttpResponse, graphql, server } from '@alphone/frontend-sdk/testing'
import { screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { expect, test, vi } from 'vitest'

import { renderAt } from './render'

vi.mock('../plugins', async (importOriginal) => {
	const wired = await importOriginal<typeof import('../plugins')>()
	return {
		plugins: [
			...wired.plugins,
			{
				id: 'probe',
				routes: () => [],
				nav: [],
				tokenAreas: [
					{ value: 'constructor', label: 'constructor' },
					{ value: 'notes', label: 'notes', writeDescription: 'Deletes stored notes.' },
				],
			},
		],
	}
})

test("ties an area's write description to its Write box only", async () => {
	server.use(graphql.query('ApiTokens', () => HttpResponse.json({ data: { apiTokens: [] } })))
	renderAt('/users/tokens/new')

	expect(await screen.findByRole('checkbox', { name: 'Write notes' })).toHaveAccessibleDescription(
		'Deletes stored notes.',
	)
	expect(screen.getByRole('checkbox', { name: 'Read notes' })).toHaveAccessibleDescription('')
})

test('asks for no plugin area left unticked, even one named like an object property', async () => {
	const seen: { scopes?: unknown } = {}
	server.use(
		graphql.query('ApiTokens', () => HttpResponse.json({ data: { apiTokens: [] } })),
		graphql.mutation('ApiTokenCreate', ({ variables }) => {
			seen.scopes = variables.scopes
			return HttpResponse.json({ data: null, errors: [{ message: 'stop here' }] })
		}),
	)
	renderAt('/users/tokens/new')

	await userEvent.type(await screen.findByLabelText('Name'), 'automation')
	expect(screen.getByRole('checkbox', { name: 'Read constructor' })).not.toBeChecked()
	await userEvent.click(screen.getByRole('checkbox', { name: 'Read tasks' }))
	await userEvent.click(screen.getByRole('button', { name: 'Create token' }))

	await vi.waitFor(() => expect(seen.scopes).toEqual(['tasks:read']))
})
