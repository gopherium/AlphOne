// SPDX-License-Identifier: AGPL-3.0-or-later

import { HttpResponse, graphql, server } from '@alphone/frontend-sdk/testing'
import { screen, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { beforeEach, expect, test } from 'vitest'

import { renderAt } from './render'

const tokenID = '0198b2f0-0000-7000-8000-0000000004a1'

/**
 * Returns one token node the listing serves.
 * @param overrides - The fields this token differs by.
 * @returns The token node.
 */
function tokenNode(overrides: Record<string, unknown> = {}) {
	return {
		__typename: 'ApiToken',
		id: tokenID,
		name: 'n8n production',
		scopes: ['contacts:read', 'tasks:write'],
		createdAt: '2026-07-06T10:00:00Z',
		lastUsedAt: '2026-08-01T09:30:00Z',
		expiresAt: '2026-11-04T10:00:00Z',
		...overrides,
	}
}

/**
 * Serves the token listing with the given tokens.
 * @param tokens - The tokens the listing answers.
 */
function servingTokens(tokens: unknown[]) {
	server.use(
		graphql.query('ApiTokens', () => HttpResponse.json({ data: { apiTokens: tokens } })),
	)
}

beforeEach(() => servingTokens([tokenNode()]))

test('ghosts the rows while the tokens arrive', async () => {
	server.use(graphql.query('ApiTokens', () => new Promise(() => {})))
	renderAt('/users/tokens')

	const status = await screen.findByRole('status')
	expect(status).toHaveTextContent('Loading tokens…')
})

test('serves the tokens screen at /users/tokens', async () => {
	renderAt('/users/tokens')

	expect(await screen.findByRole('heading', { level: 1, name: 'API tokens' })).toBeInTheDocument()
})

test('shows every token with its scopes and dates', async () => {
	renderAt('/users/tokens')

	const row = await screen.findByRole('row', { name: /n8n production/ })
	expect(within(row).getByText('contacts:read tasks:write')).toBeInTheDocument()
	expect(within(row).getByText('Nov 4, 2026')).toBeInTheDocument()
})

test('says plainly when a token never expires', async () => {
	servingTokens([tokenNode({ expiresAt: null, lastUsedAt: null })])
	renderAt('/users/tokens')

	const row = await screen.findByRole('row', { name: /n8n production/ })
	expect(within(row).getByText('Never expires')).toBeInTheDocument()
	expect(within(row).getByText('Never used')).toBeInTheDocument()
})

test('warns about a token that has already lapsed', async () => {
	servingTokens([tokenNode({ expiresAt: '2026-01-01T10:00:00Z' })])
	renderAt('/users/tokens')

	const row = await screen.findByRole('row', { name: /n8n production/ })
	expect(within(row).getByText('Expired')).toBeInTheDocument()
})

test('reports when the tokens cannot be loaded', async () => {
	server.use(
		graphql.query('ApiTokens', () =>
			HttpResponse.json({ data: null, errors: [{ message: 'internal error' }] }),
		),
	)
	renderAt('/users/tokens')

	expect(await screen.findByRole('alert')).toHaveTextContent('Tokens could not be loaded.')
})

test('shows an empty state when no token exists', async () => {
	servingTokens([])
	renderAt('/users/tokens')

	expect(await screen.findByText('No API tokens yet.')).toBeInTheDocument()
})

test('reaches the tokens screen from the users screen', async () => {
	server.use(graphql.query('Users', () => HttpResponse.json({ data: { users: [] } })))
	renderAt('/users')

	await userEvent.click(await screen.findByRole('link', { name: 'API tokens' }))

	expect(await screen.findByRole('heading', { level: 1, name: 'API tokens' })).toBeInTheDocument()
})

test('revokes a token and drops it from the list', async () => {
	let revoked: unknown = null
	let listed = [tokenNode()]
	server.use(
		graphql.query('ApiTokens', () => HttpResponse.json({ data: { apiTokens: listed } })),
		graphql.mutation('ApiTokenRevoke', ({ variables }) => {
			revoked = variables.id
			listed = []
			return HttpResponse.json({ data: { apiTokenRevoke: true } })
		}),
	)
	renderAt('/users/tokens')

	await userEvent.click(await screen.findByRole('button', { name: 'Revoke n8n production' }))

	expect(await screen.findByText('No API tokens yet.')).toBeInTheDocument()
	expect(revoked).toBe(tokenID)
})

test('reports when a token cannot be revoked', async () => {
	server.use(
		graphql.mutation('ApiTokenRevoke', () =>
			HttpResponse.json({ data: null, errors: [{ message: 'internal error' }] }),
		),
	)
	renderAt('/users/tokens')

	await userEvent.click(await screen.findByRole('button', { name: 'Revoke n8n production' }))

	expect(await screen.findByText('Revoke failed.')).toBeInTheDocument()
})
