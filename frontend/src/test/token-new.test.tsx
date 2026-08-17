// SPDX-License-Identifier: AGPL-3.0-or-later

import { HttpResponse, graphql, server } from '@alphone/frontend-sdk/testing'
import { screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { beforeEach, expect, test, vi } from 'vitest'

import { renderAt } from './render'

const secret = 'a1_dGVzdHNlY3JldHZhbHVlZm9ydGhlbWludGZvcm0'

/**
 * Serves an empty token listing so the new token screen stands alone.
 */
function servingNoTokens() {
	server.use(graphql.query('ApiTokens', () => HttpResponse.json({ data: { apiTokens: [] } })))
}

/**
 * Serves a successful mint answering the fixed secret.
 * @returns The variables the mint was asked with.
 */
function servingMint(): { asked: Record<string, unknown> | null } {
	const seen: { asked: Record<string, unknown> | null } = { asked: null }
	server.use(
		graphql.mutation('ApiTokenCreate', ({ variables }) => {
			seen.asked = variables
			return HttpResponse.json({
				data: {
					apiTokenCreate: {
						__typename: 'ApiTokenSecret',
						secret,
						token: {
							__typename: 'ApiToken',
							id: '0198b2f0-0000-7000-8000-0000000004b2',
							name: String(variables.name),
							scopes: variables.scopes,
							createdAt: '2026-08-17T10:00:00Z',
							lastUsedAt: null,
							expiresAt: '2026-09-16T10:00:00Z',
						},
					},
				},
			})
		}),
	)
	return seen
}

/**
 * Fills the mint form with a name and one granted area.
 */
async function fillMintForm() {
	await userEvent.type(await screen.findByLabelText('Name'), 'automation')
	await userEvent.click(screen.getByRole('checkbox', { name: 'Read tasks' }))
}

beforeEach(servingNoTokens)

test('serves the new token screen at /users/tokens/new', async () => {
	renderAt('/users/tokens/new')

	expect(await screen.findByRole('heading', { level: 1, name: 'New API token' })).toBeInTheDocument()
})

test('refuses to mint until a name and an area are given', async () => {
	renderAt('/users/tokens/new')

	const mint = await screen.findByRole('button', { name: 'Create token' })
	expect(mint).toHaveAttribute('aria-disabled', 'true')

	await userEvent.type(await screen.findByLabelText('Name'), 'automation')
	expect(mint).toHaveAttribute('aria-disabled', 'true')

	await userEvent.click(screen.getByRole('checkbox', { name: 'Read tasks' }))
	expect(mint).not.toHaveAttribute('aria-disabled', 'true')
})

test('gives an area back when its box is unticked', async () => {
	renderAt('/users/tokens/new')

	await userEvent.type(await screen.findByLabelText('Name'), 'automation')
	const readTasks = screen.getByRole('checkbox', { name: 'Read tasks' })
	await userEvent.click(readTasks)
	const mint = screen.getByRole('button', { name: 'Create token' })
	expect(mint).not.toHaveAttribute('aria-disabled', 'true')

	await userEvent.click(readTasks)

	expect(readTasks).not.toBeChecked()
	expect(mint).toHaveAttribute('aria-disabled', 'true')
})

test('mints the token with the areas ticked and the lifetime chosen', async () => {
	const seen = servingMint()
	renderAt('/users/tokens/new')

	await fillMintForm()
	await userEvent.click(screen.getByRole('button', { name: 'Create token' }))

	await screen.findByText(secret)
	expect(seen.asked).toEqual({ name: 'automation', scopes: ['tasks:read'], ttlDays: 90 })
})

test('grants writing without ticking reading, because writing implies it', async () => {
	const seen = servingMint()
	renderAt('/users/tokens/new')

	await userEvent.type(await screen.findByLabelText('Name'), 'automation')
	await userEvent.click(screen.getByRole('checkbox', { name: 'Write tasks' }))
	await userEvent.click(screen.getByRole('button', { name: 'Create token' }))

	await screen.findByText(secret)
	expect(seen.asked?.scopes).toEqual(['tasks:write'])
})

test('mints a token that never expires when asked', async () => {
	const seen = servingMint()
	renderAt('/users/tokens/new')

	await fillMintForm()
	await userEvent.selectOptions(screen.getByLabelText('Expires'), 'never')
	await userEvent.click(screen.getByRole('button', { name: 'Create token' }))

	await screen.findByText(secret)
	expect(seen.asked?.ttlDays).toBeNull()
})

test('shows the secret once and warns it is never shown again', async () => {
	servingMint()
	renderAt('/users/tokens/new')

	await fillMintForm()
	await userEvent.click(screen.getByRole('button', { name: 'Create token' }))

	expect(await screen.findByText(secret)).toBeInTheDocument()
	expect(screen.getByText(/never shown again/)).toBeInTheDocument()
	expect(screen.queryByRole('button', { name: 'Create token' })).not.toBeInTheDocument()
})

test('copies the secret to the clipboard', async () => {
	const writeText = vi.fn().mockResolvedValue(undefined)
	vi.stubGlobal('navigator', { ...navigator, clipboard: { writeText } })
	servingMint()
	renderAt('/users/tokens/new')

	await fillMintForm()
	await userEvent.click(screen.getByRole('button', { name: 'Create token' }))
	await userEvent.click(await screen.findByRole('button', { name: 'Copy secret' }))

	expect(writeText).toHaveBeenCalledWith(secret)
	expect(await screen.findByText('Copied.')).toBeInTheDocument()
	vi.unstubAllGlobals()
})

test('says so when the clipboard refuses the secret', async () => {
	const writeText = vi.fn().mockRejectedValue(new Error('denied'))
	vi.stubGlobal('navigator', { ...navigator, clipboard: { writeText } })
	servingMint()
	renderAt('/users/tokens/new')

	await fillMintForm()
	await userEvent.click(screen.getByRole('button', { name: 'Create token' }))
	await userEvent.click(await screen.findByRole('button', { name: 'Copy secret' }))

	expect(await screen.findByText('Copy it by hand.')).toBeInTheDocument()
	vi.unstubAllGlobals()
})

test('surfaces a rejection message from the backend', async () => {
	server.use(
		graphql.mutation('ApiTokenCreate', () =>
			HttpResponse.json({
				data: null,
				errors: [{ message: 'apitoken: malformed scope', extensions: { code: 'VALIDATION' } }],
			}),
		),
	)
	renderAt('/users/tokens/new')

	await fillMintForm()
	await userEvent.click(screen.getByRole('button', { name: 'Create token' }))

	expect(await screen.findByText('apitoken: malformed scope')).toBeInTheDocument()
})

test('falls back to generic copy when the mint fails otherwise', async () => {
	server.use(
		graphql.mutation('ApiTokenCreate', () =>
			HttpResponse.json({ data: null, errors: [{ message: 'internal error' }] }),
		),
	)
	renderAt('/users/tokens/new')

	await fillMintForm()
	await userEvent.click(screen.getByRole('button', { name: 'Create token' }))

	expect(await screen.findByText('The token could not be created.')).toBeInTheDocument()
})
