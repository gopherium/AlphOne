// SPDX-License-Identifier: AGPL-3.0-or-later

import { MANAGE_USERS } from '@alphone/frontend-sdk'
import { memberSession } from '@alphone/frontend-sdk/testing'
import { screen, within } from '@testing-library/react'
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
				accountLinks: [
					{
						label: 'Chat with support',
						href: 'https://chat.example.com/support',
						capability: MANAGE_USERS,
						openInNewTab: true,
					},
					{ label: 'Email support', href: 'mailto:support@example.com', capability: MANAGE_USERS },
					{ label: 'Status page', href: 'https://status.example.com/' },
					{ label: 'Service history', href: 'https://status.example.com/', capability: MANAGE_USERS },
				],
			},
		],
	}
})

/**
 * Returns the sidebar footer once the account panel shows in it.
 * @returns The footer element.
 */
async function sidebarFooter(): Promise<HTMLElement> {
	const logout = await screen.findByRole('button', { name: 'Log out' })
	return logout.closest('.godmin-sidebar__footer') as HTMLElement
}

test('shows a plugin link under the account panel to an account holding its capability', async () => {
	renderAt('/')

	const footer = await sidebarFooter()
	const link = within(footer).getByRole('link', { name: /^Email support/ })
	expect(link).toHaveAttribute('href', 'mailto:support@example.com')
	const logout = within(footer).getByRole('button', { name: 'Log out' })
	expect(logout.compareDocumentPosition(link) & Node.DOCUMENT_POSITION_FOLLOWING).toBeTruthy()
})

test('lists the account links in the order the plugin gives them', async () => {
	renderAt('/')

	const footer = await sidebarFooter()
	expect(within(footer).getAllByRole('link').map((link) => link.getAttribute('href'))).toEqual([
		'https://chat.example.com/support',
		'mailto:support@example.com',
		'https://status.example.com/',
		'https://status.example.com/',
	])
})

test('keeps two links to one address apart', async () => {
	const logged = vi.spyOn(console, 'error').mockImplementation(() => {})
	renderAt('/')

	const footer = await sidebarFooter()
	expect(within(footer).getByRole('link', { name: 'Status page' })).toBeInTheDocument()
	expect(within(footer).getByRole('link', { name: 'Service history' })).toBeInTheDocument()
	expect(logged.mock.calls.flat().join(' ')).not.toMatch(/same key/)
	logged.mockRestore()
})

test('hides a plugin link from an account lacking its capability', async () => {
	renderAt('/', memberSession)

	const footer = await sidebarFooter()
	expect(within(footer).queryByRole('link', { name: /^Chat with support/ })).toBeNull()
	expect(within(footer).queryByRole('link', { name: /^Email support/ })).toBeNull()
})

test('shows a link naming no capability to every signed-in account', async () => {
	renderAt('/', memberSession)

	const footer = await sidebarFooter()
	expect(within(footer).getByRole('link', { name: /^Status page/ })).toHaveAttribute(
		'href',
		'https://status.example.com/',
	)
})

test('shows no account link without a signed-in account', async () => {
	renderAt('/', null)

	await screen.findByRole('heading', { level: 1, name: 'AlphOne' })
	expect(screen.queryByRole('link', { name: /^Status page/ })).toBeNull()
})

test('opens a link asking for a new tab in one, and tells a screen reader so', async () => {
	renderAt('/')

	const footer = await sidebarFooter()
	const link = within(footer).getByRole('link', { name: 'Chat with support(opens in a new tab)' })
	expect(link).toHaveAttribute('target', '_blank')
})

test('opens any other account link in the same tab', async () => {
	renderAt('/')

	const footer = await sidebarFooter()
	const link = within(footer).getByRole('link', { name: 'Email support' })
	expect(link).not.toHaveAttribute('target')
})

test('sends no referrer with an account link', async () => {
	renderAt('/')

	const footer = await sidebarFooter()
	for (const link of within(footer).getAllByRole('link')) {
		expect(link.getAttribute('rel')?.split(' ')).toContain('noreferrer')
	}
})
