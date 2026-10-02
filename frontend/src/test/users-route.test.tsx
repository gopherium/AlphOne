// SPDX-License-Identifier: AGPL-3.0-or-later

import { rememberFormatLocale } from '@alphone/frontend-sdk'
import { HttpResponse, delay, graphql, memberSession, paging, server } from '@alphone/frontend-sdk/testing'
import { setViewport } from '@gopherium/godmin/testing'
import { resetLocale } from '@gopherium/gottext/testing'
import { screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { beforeAll, beforeEach, expect, onTestFinished, test } from 'vitest'

import { sessionQueryKey } from '@gopherium/react-auth'
import { defaultUser } from '@gopherium/react-auth/testing'
import { configureAppErrorText } from '../i18n/errors'
import { startAppLocale } from '../i18n/start'
import { createAppRouter } from '../router'
import { badgeClasses, buttonClasses, renderAt } from './render'

/** account is one user as the users query answers it. */
interface account {
	__typename: 'User'
	id: string
	email: string
	name: string
	disabled: boolean
	confirmed: boolean
	createdAt: string
	role: string
}

/**
 * Builds one account the users query answers.
 * @param id - The last hex digits of its identifier.
 * @param name - Its display name.
 * @param changes - The fields that differ from an active member.
 * @returns The account.
 */
function accountOf(id: string, name: string, changes: Partial<account> = {}): account {
	return {
		__typename: 'User',
		id: `0198b2f0-0000-7000-8000-0000000000${id}`,
		email: `${name.split(' ')[0].toLowerCase()}@example.com`,
		name,
		disabled: false,
		confirmed: true,
		createdAt: '2026-07-07T10:00:00Z',
		role: 'member',
		...changes,
	}
}

const signedIn = accountOf('01', defaultUser.name, {
	id: defaultUser.id,
	email: defaultUser.email,
	role: 'admin',
	createdAt: '2026-07-06T10:00:00Z',
})
const colleague = accountOf('ff', 'Ada Lovelace')
const barred = accountOf('fe', 'Ana Lopez', { disabled: true, createdAt: '2026-07-08T10:00:00Z' })

/** held is the account list the users query answers, changed by the mutations. */
let held: account[] = []

/**
 * Serves the given accounts from the users query.
 * @param accounts - The accounts the list holds.
 */
function holding(accounts: account[]) {
	held = accounts
	server.use(graphql.query('Users', () => HttpResponse.json({ data: { users: held } })))
}

/**
 * Returns the table row holding the given text.
 * @param text - A name or address the row shows.
 * @returns The row queries.
 */
async function rowOf(text: string) {
	return within(await screen.findByRole('row', { name: new RegExp(text) }))
}

/**
 * Returns the names the list shows, in order.
 * @returns The names.
 */
async function shownNames() {
	await screen.findByRole('table')
	return screen
		.getAllByRole('row')
		.slice(1)
		.map((row) => [signedIn, colleague, barred].find((user) => row.textContent?.includes(user.email))?.name)
}

/**
 * Opens the row actions of the named account and picks one.
 * @param name - The account the row shows.
 * @param action - The label of the action to pick.
 */
async function act(name: string, action: string) {
	const row = await rowOf(name)
	await userEvent.click(row.getByRole('button', { name: 'Actions' }))
	await userEvent.click(await screen.findByRole('menuitem', { name: action }))
}

beforeAll(async () => {
	await import('../users/UsersScreen')
})

beforeEach(() => holding([signedIn, colleague, barred]))

test('keeps in the address only what a list view can hold', () => {
	const { validateSearch } = createAppRouter().routesById['/users'].options

	const kept = (validateSearch as (raw: Record<string, unknown>) => unknown)({
		search: 'ada',
		page: 'abc',
		order: 'up',
		filters: 'status',
		tab: 'other',
	})

	expect(kept).toEqual({ search: 'ada' })
})

test('keeps the list toolbar while the accounts arrive', async () => {
	server.use(graphql.query('Users', () => new Promise(() => {})))
	renderAt('/users')

	expect(await screen.findByRole('searchbox', { name: 'Search users…' })).toBeInTheDocument()
	expect(screen.queryByRole('row')).not.toBeInTheDocument()
})

test('serves the users screen at /users', async () => {
	renderAt('/users')

	expect(await screen.findByRole('heading', { level: 1, name: 'Users' })).toBeInTheDocument()
	expect((await rowOf('Ada Lovelace')).getByText('ada@example.com')).toBeInTheDocument()
})

test('follows the page template like every other screen', async () => {
	renderAt('/users')

	const heading = await screen.findByRole('heading', { level: 1, name: 'Users' })
	expect(heading.closest('.godmin-page')).not.toBeNull()
	expect(await screen.findByRole('link', { name: 'New user' })).toBeInTheDocument()
})

test('says under the title what the users tab is for', async () => {
	renderAt('/users')

	const subtitle = await screen.findByText('Manage who can sign in and what they can do.')
	expect(subtitle).toHaveClass('godmin-page__subtitle')
})

test('says what the users tab is for in Spanish, under tabs read in Spanish', async () => {
	server.use(graphql.query('AppLocale', () => HttpResponse.json({ data: { locale: 'es-ES' } })))
	await startAppLocale()
	onTestFinished(() => resetLocale())
	renderAt('/users')

	const subtitle = await screen.findByText('Gestiona quién puede entrar y qué puede hacer.')
	expect(subtitle).toHaveClass('godmin-page__subtitle')
	const tabs = within(screen.getByRole('navigation', { name: 'Secciones de usuarios' }))
	expect(tabs.getAllByRole('link').map((tab) => tab.textContent)).toEqual(['Usuarios', 'Tokens de API'])
})

test('splits the page into a Users tab and an API tokens tab, Users current', async () => {
	renderAt('/users')

	const tabs = within(await screen.findByRole('navigation', { name: 'User sections' }))
	expect(tabs.getAllByRole('link').map((tab) => tab.textContent)).toEqual(['Users', 'API tokens'])
	expect(tabs.getByRole('link', { name: 'Users' })).toHaveAttribute('aria-current', 'page')
	expect(tabs.getByRole('link', { name: 'API tokens' })).not.toHaveAttribute('aria-current')
	expect(tabs.getByRole('link', { name: 'API tokens' })).toHaveAttribute('href', '/users/tokens')
})

test('keeps the Users tab current while the address holds a search', async () => {
	renderAt('/users?search=ada&page=2')

	const tabs = within(await screen.findByRole('navigation', { name: 'User sections' }))
	expect(tabs.getByRole('link', { name: 'Users' })).toHaveAttribute('aria-current', 'page')
	expect(tabs.getByRole('link', { name: 'Users' })).toHaveClass('godmin-page-tabs__tab--current')
})

test('leaves API tokens out of the header, now that it is a tab', async () => {
	renderAt('/users')

	const header = (await screen.findByRole('heading', { level: 1, name: 'Users' })).closest('header') as HTMLElement
	await screen.findByRole('link', { name: 'New user' })
	expect(within(header).queryByRole('link', { name: 'API tokens' })).not.toBeInTheDocument()
})

test('draws New user as a compact button, as a WordPress page header does', async () => {
	renderAt('/users')

	const add = await screen.findByRole('link', { name: 'New user' })
	expect([...add.classList]).toEqual(buttonClasses('solid', 'compact'))
})

test('lines the list up with the title, out to the canvas edges', async () => {
	renderAt('/users')

	const search = await screen.findByRole('searchbox', { name: 'Search users…' })
	expect(search.closest('.godmin-page__list')).not.toBeNull()
})

test('shows an empty state when no accounts exist', async () => {
	holding([])

	renderAt('/users')

	expect(await screen.findByText('No users yet.')).toBeInTheDocument()
	expect(screen.getByText('Add one with New user.').closest('.godmin-empty')).not.toBeNull()
})

test('says nobody matched when a search finds no account', async () => {
	renderAt('/users?search=nobody')

	expect(await screen.findByText('No users found.')).toBeInTheDocument()
	expect(screen.queryByText('No users yet.')).not.toBeInTheDocument()
})

test('reads each account status as a badge', async () => {
	holding([signedIn, barred, accountOf('fd', 'Maria Perez', { confirmed: false })])
	renderAt('/users')

	expect((await rowOf('Grace Hopper')).getByText('Active')).toBeInTheDocument()
	expect((await rowOf('Ana Lopez')).getByText('Disabled')).toBeInTheDocument()
	expect((await rowOf('Maria Perez')).getByText('Invited')).toBeInTheDocument()
})

test('draws a disabled account with the outline badge, white inside a grey border', async () => {
	holding([signedIn, barred])
	renderAt('/users')

	const badge = (await rowOf('Ana Lopez')).getByText('Disabled')
	expect([...badge.classList]).toEqual(badgeClasses('none'))
})

test('reads each status in Spanish so it agrees with the user, and offers to reactivate a disabled one', async () => {
	server.use(graphql.query('AppLocale', () => HttpResponse.json({ data: { locale: 'es-ES' } })))
	await startAppLocale()
	onTestFinished(() => resetLocale())
	holding([signedIn, barred, accountOf('fd', 'Maria Perez', { confirmed: false })])
	renderAt('/users')

	expect((await rowOf('Grace Hopper')).getByText('Activo')).toBeInTheDocument()
	expect((await rowOf('Ana Lopez')).getByText('Desactivado')).toBeInTheDocument()
	expect((await rowOf('Maria Perez')).getByText('Invitado')).toBeInTheDocument()
	await userEvent.click((await rowOf('Ana Lopez')).getByRole('button', { name: 'Actions' }))
	expect(await screen.findByRole('menuitem', { name: 'Reactivar' })).toBeInTheDocument()
})

test('reads the role every account holds by its label', async () => {
	holding([signedIn, colleague, accountOf('fc', 'Maria Perez', { role: '' })])
	renderAt('/users')

	expect((await rowOf('Ada Lovelace')).getByText('Member')).toBeInTheDocument()
	expect((await rowOf('Grace Hopper')).getByText('Admin')).toBeInTheDocument()
	expect((await rowOf('Maria Perez')).getByText('No role')).toBeInTheDocument()
})

test('shows each account email under its name, in the name column', async () => {
	renderAt('/users')

	const row = await rowOf('Ada Lovelace')
	expect(row.getByText('ada@example.com').closest('td')).toBe(row.getByText('Ada Lovelace').closest('td'))
	expect(screen.queryByRole('columnheader', { name: /Email/ })).not.toBeInTheDocument()
})

test('draws an initials avatar before every name, hidden from screen readers', async () => {
	renderAt('/users')

	const avatar = (await rowOf('Ada Lovelace')).getByText('A', { selector: '.godmin-avatar' })
	expect(avatar).toHaveAttribute('aria-hidden', 'true')
	expect(avatar.parentElement).toHaveClass('dataviews-column-primary__media')
	expect((await rowOf('Grace Hopper')).getByText('G', { selector: '.godmin-avatar' })).toBeInTheDocument()
})

test('lets a reader hide the avatars through the view options, where they read as Avatar', async () => {
	renderAt('/users')
	await rowOf('Ada Lovelace')

	await userEvent.click(screen.getByRole('button', { name: 'View options' }))
	await userEvent.click(await screen.findByRole('button', { name: 'Avatar' }))

	await waitFor(async () => expect((await rowOf('Ada Lovelace')).queryByText('A')).not.toBeInTheDocument())
})

test('draws the initials avatar before every name in the list a phone shows', async () => {
	setViewport({ matches: true })
	renderAt('/users')

	const item = (await screen.findByRole('button', { name: 'Ada Lovelace' })).closest('[role="row"]') as HTMLElement
	const avatar = within(item).getByText('A', { selector: '.godmin-avatar' })
	expect(avatar.parentElement).toHaveClass('dataviews-view-list__media-wrapper')
})

test('dates every account in the page language', async () => {
	renderAt('/users')

	expect((await rowOf('Ana Lopez')).getByText('08/07/2026')).toBeInTheDocument()
})

test('sorts the accounts by when they were created, newest first, when the address names no order', async () => {
	renderAt('/users')

	expect(await shownNames()).toEqual(['Ana Lopez', 'Ada Lovelace', 'Grace Hopper'])
	expect(screen.getByRole('columnheader', { name: /Created/ })).toHaveAttribute('aria-sort', 'descending')
})

test('sorts the accounts by the field and direction the address names', async () => {
	renderAt('/users?sort=email&order=desc')

	expect(await shownNames()).toEqual(['Grace Hopper', 'Ana Lopez', 'Ada Lovelace'])
})

test('sorts the accounts by name from A to Z when the address says so', async () => {
	renderAt('/users?sort=name&order=asc')

	expect(await shownNames()).toEqual(['Ada Lovelace', 'Ana Lopez', 'Grace Hopper'])
})

test('searches the accounts by name', async () => {
	renderAt('/users')

	await userEvent.type(await screen.findByRole('searchbox', { name: 'Search users…' }), 'Lopez')

	await waitFor(async () => expect(await shownNames()).toEqual(['Ana Lopez']))
})

test('searches the accounts by email', async () => {
	renderAt('/users')

	await userEvent.type(await screen.findByRole('searchbox', { name: 'Search users…' }), 'grace@')

	await waitFor(async () => expect(await shownNames()).toEqual(['Grace Hopper']))
})

test('narrows the accounts to the statuses the address names', async () => {
	const filters = encodeURIComponent(JSON.stringify([{ field: 'status', operator: 'isAny', value: ['disabled'] }]))
	renderAt(`/users?filters=${filters}`)

	expect(await shownNames()).toEqual(['Ana Lopez'])
})

test('narrows the accounts to the roles the address names', async () => {
	const filters = encodeURIComponent(JSON.stringify([{ field: 'role', operator: 'isAny', value: ['admin'] }]))
	renderAt(`/users?filters=${filters}`)

	expect(await shownNames()).toEqual(['Grace Hopper'])
})

test('keeps the filter chips out of sight until a filter is picked, so the filter button stays live', async () => {
	renderAt('/users')

	await screen.findByRole('table')
	expect(screen.getAllByRole('button', { name: 'Status' })).toHaveLength(1)
	expect(screen.getByRole('button', { name: 'Add filter' })).not.toHaveAttribute('aria-disabled', 'true')
})

test('narrows the accounts to the status picked through the filter button', async () => {
	renderAt('/users')

	await userEvent.click(await screen.findByRole('button', { name: 'Add filter' }))
	await userEvent.click(await screen.findByRole('menuitem', { name: 'Status' }))
	await userEvent.click(await screen.findByRole('option', { name: 'Disabled' }))

	await waitFor(async () => expect(await shownNames()).toEqual(['Ana Lopez']))
})

test('offers as roles only the ones the accounts hold, by their labels from A to Z', async () => {
	holding([colleague, signedIn, barred, accountOf('fc', 'Maria Perez', { role: '' })])
	renderAt('/users?filters=' + encodeURIComponent(JSON.stringify([{ field: 'role', operator: 'isAny', value: [] }])))

	await userEvent.click(await screen.findByRole('button', { name: 'Filter' }))
	await userEvent.click(await screen.findByRole('button', { name: 'Role', pressed: false }))

	const offered = (await screen.findAllByRole('option')).map((option) => option.textContent)
	expect(offered).toEqual(['Admin', 'Member', 'No role'])
})

test('pages the accounts by the page size the admin settings name', async () => {
	paging([2, 4], 2)
	renderAt('/users')

	expect(await shownNames()).toEqual(['Ana Lopez', 'Ada Lovelace'])
	await userEvent.click(screen.getByRole('button', { name: 'Next page' }))

	await waitFor(async () => expect(await shownNames()).toEqual(['Grace Hopper']))
})

test('offers as page sizes the ones the admin settings name', async () => {
	paging([2, 4], 2)
	renderAt('/users')

	await screen.findByRole('table')
	await userEvent.click(screen.getByRole('button', { name: 'View options' }))

	const sizes = within(await screen.findByRole('radiogroup', { name: 'Items per page' }))
	expect(sizes.getAllByRole('radio').map((size) => size.textContent)).toEqual(['2', '4'])
})

test('keeps the page the address names while the page size is on its way', async () => {
	server.use(
		graphql.query('AdminSettings', async () => {
			await delay(300)
			return HttpResponse.json({
				data: {
					adminSettings: {
						__typename: 'AdminSettings',
						toastMilliseconds: 6000,
						listPageSizes: [2, 4],
						listPageSize: 2,
						contactPageCap: 200,
						formatLocale: 'es-ES',
					},
				},
			})
		}),
	)
	renderAt('/users?page=2')

	await waitFor(async () => expect(await shownNames()).toEqual(['Grace Hopper']), { timeout: 3000 })
})

test('shows every account on one page when the page size could not be read', async () => {
	server.use(
		graphql.query('AdminSettings', () =>
			HttpResponse.json({ data: null, errors: [{ message: 'internal error' }] }),
		),
	)
	renderAt('/users')

	await waitFor(async () => expect(await shownNames()).toEqual(['Ana Lopez', 'Ada Lovelace', 'Grace Hopper']))
	expect(screen.queryByRole('button', { name: 'Next page' })).not.toBeInTheDocument()
})

test('counts the accounts, because the list offers bulk actions', async () => {
	renderAt('/users')

	expect(await screen.findByText('3 Items')).toBeInTheDocument()
})

test('offers no row action on the signed-in account', async () => {
	renderAt('/users')

	await rowOf('Ada Lovelace')
	expect((await rowOf('Grace Hopper')).queryByRole('button', { name: 'Actions' })).not.toBeInTheDocument()
	expect((await rowOf('Grace Hopper')).getByRole('checkbox')).toHaveAttribute('aria-disabled', 'true')
})

test('offers no enable on the signed-in account, even while it reads disabled', async () => {
	holding([{ ...signedIn, disabled: true }, colleague])
	renderAt('/users')

	await rowOf('Ada Lovelace')
	const own = await rowOf('Grace Hopper')
	expect(own.getByText('Disabled')).toBeInTheDocument()
	expect(own.queryByRole('button', { name: 'Actions' })).not.toBeInTheDocument()
})

test('offers a member no user management at all', async () => {
	renderAt('/users', memberSession)

	await rowOf('Ada Lovelace')
	expect(screen.queryByRole('link', { name: 'New user' })).not.toBeInTheDocument()
	expect(screen.queryByRole('button', { name: 'Actions' })).not.toBeInTheDocument()
	expect(screen.queryByRole('checkbox')).not.toBeInTheDocument()
	expect(screen.queryByText('3 Items')).not.toBeInTheDocument()
})

test('offers a member the API tokens tab, since every account holds its own tokens', async () => {
	renderAt('/users', memberSession)

	const tabs = within(await screen.findByRole('navigation', { name: 'User sections' }))
	expect(tabs.getByRole('link', { name: 'API tokens' })).toBeInTheDocument()
})

test('still lists the colleagues a member works with', async () => {
	renderAt('/users', memberSession)

	expect((await rowOf('Ada Lovelace')).getByText('ada@example.com')).toBeInTheDocument()
})

test('lays the accounts out as a list on a phone', async () => {
	setViewport({ matches: true })
	renderAt('/users')

	expect(await screen.findByText('Ada Lovelace')).toBeInTheDocument()
	expect(screen.queryByRole('table')).not.toBeInTheDocument()
})

test('leaves a row unselected when a tap lands on it on a phone', async () => {
	setViewport({ matches: true })
	renderAt('/users')

	const item = await screen.findByRole('button', { name: 'Ada Lovelace' })
	await userEvent.click(item)

	expect(item).toHaveAttribute('aria-pressed', 'false')
	expect(item.closest('[role="row"]')).not.toHaveClass('is-selected')
})

/**
 * Answers the disable mutation, flipping the account it names.
 * @returns The writes the mutation received.
 */
function writesDisabled() {
	const writes: { id: string; disabled: boolean }[] = []
	server.use(
		graphql.mutation('SetUserDisabled', ({ variables }) => {
			const write = { id: String(variables.id), disabled: Boolean(variables.disabled) }
			writes.push(write)
			held = held.map((user) => (user.id === write.id ? { ...user, disabled: write.disabled } : user))
			return HttpResponse.json({ data: { setUserDisabled: true } })
		}),
	)
	return writes
}

test('disables an account from its row actions and confirms it with a toast', async () => {
	const writes = writesDisabled()
	renderAt('/users')

	await act('Ada Lovelace', 'Disable')

	expect(await screen.findByText('User disabled.')).toBeInTheDocument()
	expect(writes).toEqual([{ id: colleague.id, disabled: true }])
	await waitFor(async () => expect((await rowOf('Ada Lovelace')).getByText('Disabled')).toBeInTheDocument())
})

test('enables a disabled account from its row actions and confirms it with a toast', async () => {
	const writes = writesDisabled()
	renderAt('/users')

	await act('Ana Lopez', 'Enable')

	expect(await screen.findByText('User enabled.')).toBeInTheDocument()
	expect(writes).toEqual([{ id: barred.id, disabled: false }])
})

test('shows why an account could not be disabled in the words the reason names', async () => {
	configureAppErrorText()
	server.use(
		graphql.mutation('SetUserDisabled', () =>
			HttpResponse.json({
				data: null,
				errors: [
					{
						message: 'user: the last privileged account cannot be disabled',
						extensions: { code: 'VALIDATION', reason: 'last_privileged_refused' },
					},
				],
			}),
		),
	)
	renderAt('/users')

	await act('Ada Lovelace', 'Disable')

	expect(await screen.findByRole('alert')).toHaveTextContent('Somebody must be able to manage users.')
	expect(screen.queryByText(/cannot be disabled$/)).not.toBeInTheDocument()
	expect(screen.queryByText('User disabled.')).not.toBeInTheDocument()
})

test('says plainly that an account could not be enabled when the server names no reason', async () => {
	server.use(
		graphql.mutation('SetUserDisabled', () =>
			HttpResponse.json({ data: null, errors: [{ message: 'internal error' }] }),
		),
	)
	renderAt('/users')

	await act('Ana Lopez', 'Enable')

	expect(await screen.findByRole('alert')).toHaveTextContent('The user could not be enabled.')
	expect(screen.queryByText('internal error')).not.toBeInTheDocument()
})

/**
 * Ticks the selection box of every named account.
 * @param names - The accounts to select.
 */
async function select(...names: string[]) {
	for (const name of names) {
		await userEvent.click((await rowOf(name)).getByRole('checkbox', { name }))
	}
}

test('disables every selected account at once with one toast counting them in the format locale', async () => {
	rememberFormatLocale('es-ES-u-nu-deva')
	holding([signedIn, colleague, accountOf('fd', 'Maria Perez')])
	const writes = writesDisabled()
	renderAt('/users')

	await select('Ada Lovelace', 'Maria Perez')
	await userEvent.click(await screen.findByRole('button', { name: 'Disable' }))

	expect(await screen.findByText('२ users disabled.')).toBeInTheDocument()
	expect(writes.map((write) => write.disabled)).toEqual([true, true])
})

test('offers Enable before Disable in the bulk bar, the one that locks accounts out last', async () => {
	renderAt('/users')

	await select('Ada Lovelace', 'Ana Lopez')

	const enable = await screen.findByRole('button', { name: 'Enable' })
	const disable = screen.getByRole('button', { name: 'Disable' })
	expect(enable.compareDocumentPosition(disable) & Node.DOCUMENT_POSITION_FOLLOWING).toBeTruthy()
})

test('names the accounts a bulk action could not change in one notice', async () => {
	holding([signedIn, colleague, accountOf('fd', 'Maria Perez')])
	server.use(
		graphql.mutation('SetUserDisabled', ({ variables }) =>
			variables.id === colleague.id
				? HttpResponse.json({ data: { setUserDisabled: true } })
				: HttpResponse.json({ data: null, errors: [{ message: 'internal error' }] }),
		),
	)
	renderAt('/users')

	await select('Ada Lovelace', 'Maria Perez')
	await userEvent.click(await screen.findByRole('button', { name: 'Disable' }))

	expect(await screen.findByText('1 user disabled.')).toBeInTheDocument()
	expect(await screen.findByRole('alert')).toHaveTextContent('1 user could not be disabled.')
})

test('enables every selected account at once with one toast', async () => {
	holding([signedIn, barred, accountOf('fd', 'Maria Perez', { disabled: true })])
	writesDisabled()
	renderAt('/users')

	await select('Ana Lopez', 'Maria Perez')
	await userEvent.click(await screen.findByRole('button', { name: 'Enable' }))

	expect(await screen.findByText('2 users enabled.')).toBeInTheDocument()
})

test('names the accounts a bulk enable could not change in one notice counting them in the format locale', async () => {
	rememberFormatLocale('es-ES-u-nu-deva')
	holding([signedIn, barred, accountOf('fd', 'Maria Perez', { disabled: true })])
	server.use(
		graphql.mutation('SetUserDisabled', () =>
			HttpResponse.json({ data: null, errors: [{ message: 'internal error' }] }),
		),
	)
	renderAt('/users')

	await select('Ana Lopez', 'Maria Perez')
	await userEvent.click(await screen.findByRole('button', { name: 'Enable' }))

	expect(await screen.findByRole('alert')).toHaveTextContent('२ users could not be enabled.')
})

test('clears an earlier failure once the next action starts', async () => {
	server.use(
		graphql.mutation('SetUserDisabled', () =>
			HttpResponse.json({ data: null, errors: [{ message: 'internal error' }] }),
		),
	)
	renderAt('/users')
	await act('Ada Lovelace', 'Disable')
	expect(await screen.findByRole('alert')).toHaveTextContent('The user could not be disabled.')
	writesDisabled()

	await act('Ada Lovelace', 'Disable')

	expect(await screen.findByText('User disabled.')).toBeInTheDocument()
	expect(screen.queryByRole('alert')).not.toBeInTheDocument()
})

/**
 * Answers the role mutation, recording the role each write asked for and giving it to the account it names.
 * @returns The roles the mutation received.
 */
function writesRole() {
	const asked: string[] = []
	server.use(
		graphql.mutation('SetUserRole', ({ variables }) => {
			const role = String(variables.role)
			asked.push(role)
			held = held.map((user) => (user.id === String(variables.id) ? { ...user, role } : user))
			return HttpResponse.json({ data: { setUserRole: true } })
		}),
	)
	return asked
}

test('changes the role of another account in a modal and confirms it with a toast', async () => {
	const asked = writesRole()
	renderAt('/users')

	await act('Ada Lovelace', 'Change role')
	const dialog = within(await screen.findByRole('dialog', { name: 'Change role' }))
	await userEvent.click(dialog.getByRole('combobox', { name: 'Role' }))
	await userEvent.click(await screen.findByRole('option', { name: 'Admin' }))
	await userEvent.click(dialog.getByRole('button', { name: 'Change role' }))

	expect(await screen.findByText('Role changed.')).toBeInTheDocument()
	expect(asked).toEqual(['admin'])
	await waitFor(() => expect(screen.queryByRole('dialog')).not.toBeInTheDocument())
	expect((await rowOf('Ada Lovelace')).getByText('Admin')).toBeInTheDocument()
})

test('offers only the roles the reader may grant', async () => {
	renderAt('/users')

	await act('Ada Lovelace', 'Change role')
	await userEvent.click(await screen.findByRole('combobox', { name: 'Role' }))

	const offered = (await screen.findAllByRole('option')).map((option) => option.textContent)
	expect(offered).toEqual(['Admin', 'Member'])
})

test('lifts the role choices into the overlay slot that stacks above the modal', async () => {
	renderAt('/users')

	await act('Ada Lovelace', 'Change role')
	await userEvent.click(await screen.findByRole('combobox', { name: 'Role' }))

	const choice = await screen.findByRole('option', { name: 'Admin' })
	expect(choice.closest('[data-wp-compat-overlay-slot]')).not.toBeNull()
})

test('opens the role change in a small modal naming the account, with Cancel as a text button', async () => {
	renderAt('/users')

	await act('Ada Lovelace', 'Change role')

	const dialog = await screen.findByRole('dialog', { name: 'Change role' })
	expect(dialog).toHaveClass('has-size-small')
	expect(within(dialog).getByText('Choose the role of Ada Lovelace.')).toBeInTheDocument()
	expect([...within(dialog).getByRole('button', { name: 'Cancel' }).classList]).toEqual(buttonClasses('minimal'))
})

test('waits for another role before it writes one', async () => {
	renderAt('/users')

	await act('Ada Lovelace', 'Change role')

	expect(await screen.findByRole('button', { name: 'Change role' })).toHaveAttribute('aria-disabled', 'true')
})

test('closes the role modal without writing when the reader cancels', async () => {
	const asked = writesRole()
	renderAt('/users')

	await act('Ada Lovelace', 'Change role')
	await userEvent.click(await screen.findByRole('button', { name: 'Cancel' }))

	await waitFor(() => expect(screen.queryByRole('dialog')).not.toBeInTheDocument())
	expect(asked).toEqual([])
})

test('shows why a role could not be changed inside the modal', async () => {
	configureAppErrorText()
	server.use(
		graphql.mutation('SetUserRole', () =>
			HttpResponse.json({
				data: null,
				errors: [
					{
						message: 'user: the role outranks the caller',
						extensions: { code: 'VALIDATION', reason: 'role_beyond_reach' },
					},
				],
			}),
		),
	)
	renderAt('/users')

	await act('Ada Lovelace', 'Change role')
	const dialog = within(await screen.findByRole('dialog'))
	await userEvent.click(dialog.getByRole('combobox', { name: 'Role' }))
	await userEvent.click(await screen.findByRole('option', { name: 'Admin' }))
	await userEvent.click(dialog.getByRole('button', { name: 'Change role' }))

	expect(await dialog.findByRole('alert')).toHaveTextContent('That role holds more than your own.')
	expect(screen.queryByText('Role changed.')).not.toBeInTheDocument()
})

test('says plainly that a role could not be changed when the server names no reason', async () => {
	server.use(
		graphql.mutation('SetUserRole', () =>
			HttpResponse.json({ data: null, errors: [{ message: 'internal error' }] }),
		),
	)
	renderAt('/users')

	await act('Ada Lovelace', 'Change role')
	const dialog = within(await screen.findByRole('dialog'))
	await userEvent.click(dialog.getByRole('combobox', { name: 'Role' }))
	await userEvent.click(await screen.findByRole('option', { name: 'Admin' }))
	await userEvent.click(dialog.getByRole('button', { name: 'Change role' }))

	expect(await dialog.findByRole('alert')).toHaveTextContent('The role could not be changed.')
})

test('navigates to the users screen from the main menu', async () => {
	renderAt('/')

	await userEvent.click(await screen.findByRole('link', { name: 'Users' }))

	expect(await screen.findByRole('heading', { name: 'Users' })).toBeInTheDocument()
})

/**
 * Fills the new user form and submits it.
 */
async function submitNewUser() {
	await userEvent.type(await screen.findByLabelText('Email'), 'ada@example.com')
	await userEvent.type(screen.getByLabelText('Name'), 'Ada Lovelace')
	expect(screen.queryByLabelText('Password')).not.toBeInTheDocument()
	await userEvent.click(screen.getByRole('button', { name: 'Send invitation' }))
}

test('returns to the user list after an invitation went out, confirming it with a toast', async () => {
	server.use(
		graphql.mutation('Invite', () =>
			HttpResponse.json({
				data: { invite: { __typename: 'InvitePayload', delivered: true, activationLink: null } },
			}),
		),
	)
	renderAt('/users/new')

	await submitNewUser()

	expect(await screen.findByRole('heading', { name: 'Users' })).toBeInTheDocument()
	expect(await screen.findByText('Invitation sent.')).toBeInTheDocument()
})

test('leaves the invitation screen once the link is delivered by hand, without a toast', async () => {
	server.use(
		graphql.mutation('Invite', () =>
			HttpResponse.json({
				data: {
					invite: {
						__typename: 'InvitePayload',
						delivered: false,
						activationLink: '/activate?token=t-5',
					},
				},
			}),
		),
	)
	renderAt('/users/new')
	await submitNewUser()
	await screen.findByLabelText('Activation link')

	await userEvent.click(screen.getByRole('button', { name: 'Done' }))

	expect(await screen.findByRole('heading', { name: 'Users' })).toBeInTheDocument()
	expect(screen.queryByText('Invitation sent.')).not.toBeInTheDocument()
})

test('shows the activation link when no mail server delivered the invitation', async () => {
	server.use(
		graphql.mutation('Invite', () =>
			HttpResponse.json({
				data: {
					invite: {
						__typename: 'InvitePayload',
						delivered: false,
						activationLink: '/activate?token=t-4',
					},
				},
			}),
		),
	)
	renderAt('/users/new')

	await submitNewUser()

	expect(await screen.findByLabelText('Activation link')).toHaveValue('/activate?token=t-4')
})

test('surfaces a rejection message from the backend', async () => {
	server.use(
		graphql.mutation('Invite', () =>
			HttpResponse.json({
				data: null,
				errors: [{ message: 'that name is too long', extensions: { code: 'VALIDATION' } }],
			}),
		),
	)
	renderAt('/users/new')

	await submitNewUser()

	expect(await screen.findByText('that name is too long')).toBeInTheDocument()
})

test('falls back to generic copy when the invitation fails otherwise', async () => {
	server.use(
		graphql.mutation('Invite', () =>
			HttpResponse.json({ data: null, errors: [{ message: 'internal error' }] }),
		),
	)
	renderAt('/users/new')

	await submitNewUser()

	expect(await screen.findByText('The invitation could not be sent.')).toBeInTheDocument()
})

test('drops the session when the users request is unauthorized', async () => {
	server.use(
		graphql.query('Users', () =>
			HttpResponse.json({
				data: null,
				errors: [{ message: 'no session', extensions: { code: 'UNAUTHENTICATED' } }],
			}),
		),
	)

	const client = renderAt('/users')

	await waitFor(() => expect(client.getQueryData(sessionQueryKey)).toBeNull())
})

test('keeps the session when the users request fails for other reasons', async () => {
	server.use(
		graphql.query('Users', () =>
			HttpResponse.json({ data: null, errors: [{ message: 'internal error' }] }),
		),
	)

	const client = renderAt('/users')

	expect(await screen.findByRole('alert')).toHaveTextContent('Users could not be loaded.')
	expect(client.getQueryData(sessionQueryKey)).not.toBeNull()
})
