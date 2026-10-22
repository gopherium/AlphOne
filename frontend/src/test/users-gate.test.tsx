// SPDX-License-Identifier: AGPL-3.0-or-later

import { HttpResponse, graphql, memberSession, server } from '@alphone/frontend-sdk/testing'
import { resetLocale } from '@gopherium/gottext/testing'
import { screen, within } from '@testing-library/react'
import { beforeAll, expect, onTestFinished, test } from 'vitest'

import { startAppLocale } from '../i18n/start'
import { renderAt } from './render'

/**
 * Answers the users query with no account, counting every request.
 * @returns The count of users requests the app sent.
 */
function countedUsers() {
	const asked = { count: 0 }
	server.use(
		graphql.query('Users', () => {
			asked.count += 1
			return HttpResponse.json({ data: { users: [] } })
		}),
	)
	return asked
}

beforeAll(async () => {
	await import('../users/UsersScreen')
	await import('../users/TokensScreen')
})

test('sends a member home from the Users screen without asking for the accounts', async () => {
	const asked = countedUsers()
	renderAt('/users', memberSession)

	expect(await screen.findByRole('heading', { level: 1, name: 'Tasks' })).toBeInTheDocument()
	expect(screen.queryByRole('searchbox', { name: 'Search users…' })).not.toBeInTheDocument()
	expect(asked.count).toBe(0)
})

test('sends a member home from the New user screen', async () => {
	renderAt('/users/new', memberSession)

	expect(await screen.findByRole('heading', { level: 1, name: 'Tasks' })).toBeInTheDocument()
	expect(screen.queryByRole('button', { name: 'Send invitation' })).not.toBeInTheDocument()
})

test('leaves Users out of a member menu and offers API tokens in its place', async () => {
	renderAt('/', memberSession)

	const menu = within(await screen.findByRole('navigation', { name: 'Navigation' }))
	const names = menu.getAllByRole('link').map((link) => link.textContent)
	expect(names.slice(0, 4)).toEqual(['Tasks', 'Contacts', 'API tokens', 'Language'])
	expect(menu.queryByRole('link', { name: 'Users' })).not.toBeInTheDocument()
	expect(menu.getByRole('link', { name: 'API tokens' })).toHaveAttribute('href', '/users/tokens')
})

test('reads the API tokens entry and the title of its page in Spanish', async () => {
	server.use(graphql.query('AppLocale', () => HttpResponse.json({ data: { locale: 'es-ES' } })))
	await startAppLocale()
	onTestFinished(() => resetLocale())
	renderAt('/users/tokens', memberSession)

	expect(await screen.findByRole('heading', { level: 1, name: 'Tokens de API' })).toBeInTheDocument()
	const menu = within(screen.getByRole('navigation', { name: 'Navegación' }))
	expect(menu.getByRole('link', { name: 'Tokens de API' })).toHaveAttribute('href', '/users/tokens')
})
