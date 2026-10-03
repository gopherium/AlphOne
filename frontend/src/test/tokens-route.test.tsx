// SPDX-License-Identifier: AGPL-3.0-or-later

import { key, rememberFormatLocale } from '@alphone/frontend-sdk'
import { HttpResponse, graphql, memberSession, server } from '@alphone/frontend-sdk/testing'
import { setViewport } from '@gopherium/godmin/testing'
import { resetLocale } from '@gopherium/gottext/testing'
import { render, screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import type { ReactElement } from 'react'
import { beforeAll, beforeEach, expect, onTestFinished, test } from 'vitest'

import { configureAppErrorText } from '../i18n/errors'
import { startAppLocale } from '../i18n/start'
import { createAppRouter } from '../router'
import { buttonClasses, renderAt } from './render'

/** token is one token as the listing answers it. */
interface token {
	__typename: 'ApiToken'
	id: string
	name: string
	scopes: string[]
	createdAt: string
	lastUsedAt: string | null
	expiresAt: string | null
}

/**
 * Builds one token the listing answers.
 * @param id - The last hex digits of its identifier.
 * @param name - Its name.
 * @param changes - The fields that differ from a token used once that expires in November.
 * @returns The token.
 */
function tokenOf(id: string, name: string, changes: Partial<token> = {}): token {
	return {
		__typename: 'ApiToken',
		id: `0198b2f0-0000-7000-8000-0000000004${id}`,
		name,
		scopes: ['contacts:read', 'tasks:write'],
		createdAt: '2026-07-06T10:00:00Z',
		lastUsedAt: '2026-08-01T09:30:00Z',
		expiresAt: '2026-11-04T10:00:00Z',
		...changes,
	}
}

const production = tokenOf('a1', 'n8n production')
const sync = tokenOf('a2', 'Zapier sync', {
	scopes: ['contacts:write'],
	createdAt: '2026-07-10T10:00:00Z',
	lastUsedAt: null,
	expiresAt: null,
})
const report = tokenOf('a3', 'Monthly report', {
	scopes: ['tasks:read'],
	createdAt: '2026-07-08T10:00:00Z',
	lastUsedAt: '2026-08-20T09:30:00Z',
	expiresAt: '2026-12-01T10:00:00Z',
})

/** held is the token list the listing answers, changed by the revokes. */
let held: token[] = []

/**
 * Serves the given tokens from the listing.
 * @param tokens - The tokens the listing holds.
 */
function holding(tokens: token[]) {
	held = tokens
	server.use(graphql.query('ApiTokens', () => HttpResponse.json({ data: { apiTokens: held } })))
}

/**
 * Answers the revoke mutation, dropping the token it names from the listing.
 * @returns The tokens the mutation was asked to revoke.
 */
function revokes() {
	const asked: string[] = []
	server.use(
		graphql.mutation('ApiTokenRevoke', ({ variables }) => {
			const id = String(variables.id)
			asked.push(id)
			held = held.filter((each) => each.id !== id)
			return HttpResponse.json({ data: { apiTokenRevoke: true } })
		}),
	)
	return asked
}

/**
 * Returns the table row holding the given text.
 * @param text - A name the row shows.
 * @returns The row queries.
 */
async function rowOf(text: string) {
	return within(await screen.findByRole('row', { name: new RegExp(text) }))
}

/**
 * Returns the token names the list shows, in order.
 * @returns The names.
 */
async function shownNames() {
	await screen.findByRole('table')
	return screen
		.getAllByRole('row')
		.slice(1)
		.map((row) => [production, sync, report].find((each) => row.textContent?.includes(each.name))?.name)
}

/**
 * Opens the row actions of the named token and picks one.
 * @param name - The token the row shows.
 * @param action - The label of the action to pick.
 */
async function act(name: string, action: string) {
	const row = await rowOf(name)
	await userEvent.click(row.getByRole('button', { name: 'Actions' }))
	await userEvent.click(await screen.findByRole('menuitem', { name: action }))
}

/**
 * Ticks the selection box of every named token.
 * @param names - The tokens to select.
 */
async function select(...names: string[]) {
	for (const name of names) {
		await userEvent.click((await rowOf(name)).getByRole('checkbox', { name }))
	}
}

beforeAll(async () => {
	await import('../users/UsersScreen')
	await import('../users/TokensScreen')
})

beforeEach(() => holding([production, sync, report]))

test('keeps in the address only what a list view can hold', () => {
	const { validateSearch } = createAppRouter().routesById['/users/tokens'].options

	const kept = (validateSearch as (raw: Record<string, unknown>) => unknown)({
		search: 'n8n',
		page: 'abc',
		order: 'up',
		tab: 'other',
	})

	expect(kept).toEqual({ search: 'n8n' })
})

test('keeps the list toolbar while the tokens arrive', async () => {
	server.use(graphql.query('ApiTokens', () => new Promise(() => {})))
	renderAt('/users/tokens')

	expect(await screen.findByRole('searchbox', { name: 'Search tokens…' })).toBeInTheDocument()
	expect(screen.queryByRole('row')).not.toBeInTheDocument()
})

test('serves the tokens tab at /users/tokens, under the Users title the page keeps', async () => {
	renderAt('/users/tokens')

	expect(await screen.findByRole('heading', { level: 1, name: 'Users' })).toBeInTheDocument()
	const tabs = within(screen.getByRole('navigation', { name: 'User sections' }))
	expect(tabs.getAllByRole('link').map((tab) => tab.textContent)).toEqual(['Users', 'API tokens'])
	expect(tabs.getByRole('link', { name: 'API tokens' })).toHaveAttribute('aria-current', 'page')
	expect(tabs.getByRole('link', { name: 'API tokens' })).toHaveClass('godmin-page-tabs__tab--current')
	expect(tabs.getByRole('link', { name: 'Users' })).not.toHaveAttribute('aria-current')
	expect(tabs.getByRole('link', { name: 'Users' })).not.toHaveClass('godmin-page-tabs__tab--current')
})

test('says under the title what the tokens tab is for', async () => {
	renderAt('/users/tokens')

	const subtitle = await screen.findByText('Manage the tokens your programs sign in with.')
	expect(subtitle).toHaveClass('godmin-page__subtitle')
})

test('says what the tokens tab is for in Spanish', async () => {
	server.use(graphql.query('AppLocale', () => HttpResponse.json({ data: { locale: 'es-ES' } })))
	await startAppLocale()
	onTestFinished(() => resetLocale())
	renderAt('/users/tokens')

	const subtitle = await screen.findByText('Gestiona los tokens con los que entran tus programas.')
	expect(subtitle).toHaveClass('godmin-page__subtitle')
})

test('keeps Users current in the main menu on the tokens tab', async () => {
	renderAt('/users/tokens')

	const menu = within(await screen.findByRole('navigation', { name: 'Navigation' }))
	expect(await menu.findByRole('link', { name: 'Users' })).toHaveAttribute('aria-current', 'page')
})

test('lines the list up with the title, out to the canvas edges, as the Users tab does', async () => {
	renderAt('/users/tokens')

	const search = await screen.findByRole('searchbox', { name: 'Search tokens…' })
	expect(search.closest('.godmin-page__list')).not.toBeNull()
	expect(search.closest('.dataviews-wrapper')).not.toBeNull()
})

test('draws New token as a compact button, as a WordPress page header does', async () => {
	renderAt('/users/tokens')

	const add = await screen.findByRole('link', { name: 'New token' })
	expect([...add.classList]).toEqual(buttonClasses('solid', 'compact'))
})

test('heads the columns with the token name first and its three dates after it', async () => {
	renderAt('/users/tokens')

	await rowOf('n8n production')
	const headers = screen.getAllByRole('columnheader').map((header) => header.textContent?.replace('↓', ''))
	expect(headers.slice(1)).toEqual(['Name', 'Created', 'Last used', 'Expires', 'Actions'])
})

test('shows each token scopes under its name, in the name column, so the table fits a tablet', async () => {
	renderAt('/users/tokens')

	const row = await rowOf('n8n production')
	expect(row.getByText('contacts:read, tasks:write').closest('td')).toBe(row.getByText('n8n production').closest('td'))
	expect(screen.queryByRole('columnheader', { name: /Scopes/ })).not.toBeInTheDocument()
})

test('shows every token with its dates', async () => {
	renderAt('/users/tokens')

	const row = await rowOf('n8n production')
	expect(row.getByText('06/07/2026')).toBeInTheDocument()
	expect(row.getByText('01/08/2026')).toBeInTheDocument()
	expect(row.getByText('04/11/2026')).toBeInTheDocument()
})

test('says plainly when a token never expires and was never used', async () => {
	renderAt('/users/tokens')

	const row = await rowOf('Zapier sync')
	expect(row.getByText('Never expires')).toBeInTheDocument()
	expect(row.getByText('Never used')).toBeInTheDocument()
})

test('warns about a token that has already lapsed', async () => {
	holding([tokenOf('a1', 'n8n production', { expiresAt: '2026-01-01T10:00:00Z' })])
	renderAt('/users/tokens')

	expect((await rowOf('n8n production')).getByText('Expired')).toBeInTheDocument()
})

test('sorts the tokens by when they were created, newest first, when the address names no order', async () => {
	renderAt('/users/tokens')

	expect(await shownNames()).toEqual(['Zapier sync', 'Monthly report', 'n8n production'])
	expect(screen.getByRole('columnheader', { name: /Created/ })).toHaveAttribute('aria-sort', 'descending')
})

test('sorts the tokens by name from A to Z when the address says so', async () => {
	renderAt('/users/tokens?sort=name&order=asc')

	expect(await shownNames()).toEqual(['Monthly report', 'n8n production', 'Zapier sync'])
})

test('sorts the tokens by when they were last used, the unused one last', async () => {
	renderAt('/users/tokens?sort=lastUsed&order=desc')

	expect(await shownNames()).toEqual(['Monthly report', 'n8n production', 'Zapier sync'])
})

test('sorts the tokens by when they expire, the endless one last', async () => {
	renderAt('/users/tokens?sort=expires&order=asc')

	expect(await shownNames()).toEqual(['n8n production', 'Monthly report', 'Zapier sync'])
})

test('searches the tokens by name', async () => {
	renderAt('/users/tokens')

	await userEvent.type(await screen.findByRole('searchbox', { name: 'Search tokens…' }), 'zapier')

	await waitFor(async () => expect(await shownNames()).toEqual(['Zapier sync']))
})

test('narrows the tokens to the ones holding any scope the address names', async () => {
	const filters = encodeURIComponent(
		JSON.stringify([{ field: 'scopes', operator: 'isAny', value: ['tasks:write', 'tasks:read'] }]),
	)
	renderAt(`/users/tokens?filters=${filters}`)

	expect(await shownNames()).toEqual(['Monthly report', 'n8n production'])
})

test('offers as scopes only the ones the tokens hold, from A to Z', async () => {
	const filters = encodeURIComponent(JSON.stringify([{ field: 'scopes', operator: 'isAny', value: [] }]))
	renderAt(`/users/tokens?filters=${filters}`)

	await userEvent.click(await screen.findByRole('button', { name: 'Filter' }))
	await userEvent.click(await screen.findByRole('button', { name: 'Scopes', pressed: false }))

	const offered = (await screen.findAllByRole('option')).map((option) => option.textContent)
	expect(offered).toEqual(['contacts:read', 'contacts:write', 'tasks:read', 'tasks:write'])
})

test('counts the tokens, because the list offers a bulk revoke', async () => {
	renderAt('/users/tokens')

	expect(await screen.findByText('3 Items')).toBeInTheDocument()
})

test('lays the tokens out as a list on a phone', async () => {
	setViewport({ matches: true })
	renderAt('/users/tokens')

	expect(await screen.findByText('n8n production')).toBeInTheDocument()
	expect(screen.queryByRole('table')).not.toBeInTheDocument()
})

test('reports when the tokens cannot be loaded', async () => {
	server.use(
		graphql.query('ApiTokens', () => HttpResponse.json({ data: null, errors: [{ message: 'internal error' }] })),
	)
	renderAt('/users/tokens')

	expect(await screen.findByRole('alert')).toHaveTextContent('Tokens could not be loaded.')
})

test('shows an empty state when no token exists', async () => {
	holding([])
	renderAt('/users/tokens')

	expect(await screen.findByText('No API tokens yet.')).toBeInTheDocument()
	expect(screen.getByText('Add one with New token.').closest('.godmin-empty')).not.toBeNull()
})

/**
 * Returns the drawing an icon carries.
 * @param icon - The icon element.
 * @returns The path data of its drawing.
 */
function drawingOf(icon: ReactElement): string | null | undefined {
	const { container, unmount } = render(icon)
	const drawn = container.querySelector('path')?.getAttribute('d')
	unmount()
	return drawn
}

/**
 * Returns the drawing of the icon an empty state holding the given title shows.
 * @param title - The title of the empty state.
 * @returns The path data of its icon.
 */
async function emptyDrawing(title: string): Promise<string | null | undefined> {
	const empty = (await screen.findByText(title)).closest('.godmin-empty')
	return empty?.querySelector('svg path')?.getAttribute('d')
}

test('draws a key on an empty token list', async () => {
	holding([])
	renderAt('/users/tokens')

	expect(await emptyDrawing('No API tokens yet.')).toBe(drawingOf(key))
})

test('draws a key on a token list a search narrowed to nothing', async () => {
	renderAt('/users/tokens?search=nothing')

	expect(await emptyDrawing('No tokens found.')).toBe(drawingOf(key))
})

test('says no token matched when a search finds none', async () => {
	renderAt('/users/tokens?search=nothing')

	expect(await screen.findByText('No tokens found.')).toBeInTheDocument()
	expect(screen.queryByText('No API tokens yet.')).not.toBeInTheDocument()
})

test('reaches the tokens tab from the users tab', async () => {
	server.use(graphql.query('Users', () => HttpResponse.json({ data: { users: [] } })))
	renderAt('/users')
	const tabs = within(await screen.findByRole('navigation', { name: 'User sections' }))

	await userEvent.click(tabs.getByRole('link', { name: 'API tokens' }))

	expect(await screen.findByRole('row', { name: /n8n production/ })).toBeInTheDocument()
	const shown = within(screen.getByRole('navigation', { name: 'User sections' }))
	expect(shown.getByRole('link', { name: 'API tokens' })).toHaveAttribute('aria-current', 'page')
	expect(screen.getByRole('heading', { level: 1, name: 'Users' })).toBeInTheDocument()
})

test('returns to the users tab from the tokens tab', async () => {
	server.use(graphql.query('Users', () => HttpResponse.json({ data: { users: [] } })))
	renderAt('/users/tokens')
	const tabs = within(await screen.findByRole('navigation', { name: 'User sections' }))

	await userEvent.click(tabs.getByRole('link', { name: 'Users' }))

	expect(await screen.findByText('No users yet.')).toBeInTheDocument()
	const shown = within(screen.getByRole('navigation', { name: 'User sections' }))
	expect(shown.getByRole('link', { name: 'Users' })).toHaveAttribute('aria-current', 'page')
	expect(shown.getByRole('link', { name: 'API tokens' })).not.toHaveAttribute('aria-current')
})

test('keeps Revoke in the row actions menu, never as a button in the row', async () => {
	renderAt('/users/tokens')

	const row = await rowOf('n8n production')
	expect(row.queryByRole('button', { name: /Revoke/ })).not.toBeInTheDocument()
	await userEvent.click(row.getByRole('button', { name: 'Actions' }))
	expect(await screen.findByRole('menuitem', { name: 'Revoke' })).toBeInTheDocument()
})

test('offers a member the revoke of the tokens the member holds', async () => {
	renderAt('/users/tokens', memberSession)

	await userEvent.click((await rowOf('n8n production')).getByRole('button', { name: 'Actions' }))
	expect(await screen.findByRole('menuitem', { name: 'Revoke' })).toBeInTheDocument()
})

test('asks before it revokes, in a small modal naming the token, with Cancel as a text button', async () => {
	renderAt('/users/tokens')

	await act('n8n production', 'Revoke')

	const dialog = await screen.findByRole('dialog', { name: 'Revoke token' })
	expect(dialog).toHaveClass('has-size-small')
	expect(
		within(dialog).getByText('Revoke the token n8n production? Anything using it stops working at once.'),
	).toBeInTheDocument()
	expect([...within(dialog).getByRole('button', { name: 'Cancel' }).classList]).toEqual(buttonClasses('minimal'))
})

test('closes the revoke modal without revoking when the reader cancels', async () => {
	const asked = revokes()
	renderAt('/users/tokens')

	await act('n8n production', 'Revoke')
	await userEvent.click(await screen.findByRole('button', { name: 'Cancel' }))

	await waitFor(() => expect(screen.queryByRole('dialog')).not.toBeInTheDocument())
	expect(asked).toEqual([])
})

test('revokes a token once confirmed, drops it from the list and confirms it with a toast', async () => {
	const asked = revokes()
	renderAt('/users/tokens')

	await act('n8n production', 'Revoke')
	const dialog = within(await screen.findByRole('dialog', { name: 'Revoke token' }))
	await userEvent.click(dialog.getByRole('button', { name: 'Revoke' }))

	expect(await screen.findByText('Token revoked.')).toBeInTheDocument()
	expect(asked).toEqual([production.id])
	await waitFor(() => expect(screen.queryByRole('dialog')).not.toBeInTheDocument())
	await waitFor(async () => expect(await shownNames()).toEqual(['Zapier sync', 'Monthly report']))
})

test('confirms a revoke with a toast in Spanish', async () => {
	server.use(graphql.query('AppLocale', () => HttpResponse.json({ data: { locale: 'es-ES' } })))
	await startAppLocale()
	onTestFinished(() => resetLocale())
	revokes()
	renderAt('/users/tokens')

	await act('n8n production', 'Revocar')
	const dialog = within(await screen.findByRole('dialog', { name: 'Revocar token' }))
	await userEvent.click(dialog.getByRole('button', { name: 'Revocar' }))

	expect(await screen.findByText('Token revocado.')).toBeInTheDocument()
})

test('shows why a token could not be revoked above the list, without a toast', async () => {
	configureAppErrorText()
	server.use(
		graphql.mutation('ApiTokenRevoke', () =>
			HttpResponse.json({ data: null, errors: [{ message: 'internal error' }] }),
		),
	)
	renderAt('/users/tokens')

	await act('n8n production', 'Revoke')
	await userEvent.click(within(await screen.findByRole('dialog')).getByRole('button', { name: 'Revoke' }))

	expect(await screen.findByRole('alert')).toHaveTextContent('The token could not be revoked.')
	expect(screen.queryByText('internal error')).not.toBeInTheDocument()
	expect(screen.queryByText('Token revoked.')).not.toBeInTheDocument()
	expect(screen.queryByRole('dialog')).not.toBeInTheDocument()
})

test('revokes every selected token after one question with one toast, counted in the format locale', async () => {
	rememberFormatLocale('es-ES-u-nu-deva')
	const asked = revokes()
	renderAt('/users/tokens')

	await select('n8n production', 'Zapier sync')
	await userEvent.click(await screen.findByRole('button', { name: 'Revoke' }))
	const dialog = within(await screen.findByRole('dialog', { name: 'Revoke tokens' }))
	expect(dialog.getByText('Revoke २ tokens? Anything using them stops working at once.')).toBeInTheDocument()
	await userEvent.click(dialog.getByRole('button', { name: 'Revoke' }))

	expect(await screen.findByText('२ tokens revoked.')).toBeInTheDocument()
	expect([...asked].sort()).toEqual([production.id, sync.id])
	await waitFor(async () => expect(await shownNames()).toEqual(['Monthly report']))
})

test('names the tokens a bulk revoke could not reach in one notice counting them in the format locale', async () => {
	rememberFormatLocale('es-ES-u-nu-deva')
	server.use(
		graphql.mutation('ApiTokenRevoke', ({ variables }) =>
			variables.id === production.id
				? HttpResponse.json({ data: { apiTokenRevoke: true } })
				: HttpResponse.json({ data: null, errors: [{ message: 'internal error' }] }),
		),
	)
	renderAt('/users/tokens')

	await select('n8n production', 'Zapier sync')
	await userEvent.click(await screen.findByRole('button', { name: 'Revoke' }))
	await userEvent.click(within(await screen.findByRole('dialog')).getByRole('button', { name: 'Revoke' }))

	expect(await screen.findByText('१ token revoked.')).toBeInTheDocument()
	expect(await screen.findByRole('alert')).toHaveTextContent('१ token could not be revoked.')
})

test('clears an earlier failure once the next revoke starts', async () => {
	server.use(
		graphql.mutation('ApiTokenRevoke', () =>
			HttpResponse.json({ data: null, errors: [{ message: 'internal error' }] }),
		),
	)
	renderAt('/users/tokens')
	await act('n8n production', 'Revoke')
	await userEvent.click(within(await screen.findByRole('dialog')).getByRole('button', { name: 'Revoke' }))
	expect(await screen.findByRole('alert')).toHaveTextContent('The token could not be revoked.')
	revokes()

	await act('n8n production', 'Revoke')
	await userEvent.click(within(await screen.findByRole('dialog')).getByRole('button', { name: 'Revoke' }))

	expect(await screen.findByText('Token revoked.')).toBeInTheDocument()
	expect(screen.queryByRole('alert')).not.toBeInTheDocument()
})
