// SPDX-License-Identifier: AGPL-3.0-or-later

import { HttpResponse, delay, graphql, paging, server } from '@alphone/frontend-sdk/testing'
import { setViewport } from '@gopherium/godmin/testing'
import { resetLocale } from '@gopherium/gottext/testing'
import { screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { beforeAll, beforeEach, expect, onTestFinished, test } from 'vitest'

import { sessionQueryKey } from '@gopherium/react-auth'
import { startAppLocale } from '../i18n/start'
import { createAppRouter } from '../router'
import { gate } from './gate'
import { buttonClasses, compactClasses, renderAt, renderRoutedAt } from './render'
import { accountOf, barred, colleague, rowOf, serveUsers, signedIn } from './usersScreen'
import type { account } from './usersScreen'

/**
 * Serves the given accounts from the users query.
 * @param accounts - The accounts the list holds.
 */
function holding(accounts: account[]) {
	serveUsers(() => accounts)
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
 * Opens the view options of the table and returns their popover.
 * @returns The popover queries.
 */
async function viewOptions() {
	await screen.findByRole('table')
	await userEvent.click(screen.getByRole('button', { name: 'View options' }))
	return within(
		await waitFor(() => {
			const popover = document.querySelector<HTMLElement>('.dataviews-config__popover')
			expect(popover).not.toBeNull()
			return popover as HTMLElement
		}),
	)
}

beforeAll(async () => {
	await import('../users/UsersScreen')
})

beforeEach(() => holding([signedIn, colleague, barred]))

test('keeps in the address only what a list view can hold', () => {
	const { validateSearch } = createAppRouter().routesById['/admin/users'].options

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

	expect(await screen.findByRole('heading', { level: 2, name: 'Users' })).toBeInTheDocument()
	expect((await rowOf('Ada Lovelace')).getByText('ada@example.com')).toBeInTheDocument()
})

test('follows the page template like every other screen', async () => {
	renderAt('/users')

	const heading = await screen.findByRole('heading', { level: 2, name: 'Users' })
	expect(heading.closest('.godmin-page')).not.toBeNull()
	expect(await screen.findByRole('link', { name: 'Add User' })).toBeInTheDocument()
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

	const header = (await screen.findByRole('heading', { level: 2, name: 'Users' })).closest('header') as HTMLElement
	await screen.findByRole('link', { name: 'Add User' })
	expect(within(header).queryByRole('link', { name: 'API tokens' })).not.toBeInTheDocument()
})

test('draws Add User as a compact button, as a WordPress page header does', async () => {
	renderAt('/users')

	const add = await screen.findByRole('link', { name: 'Add User' })
	expect(add).toHaveAttribute('href', '/users/new')
	expect(add).toHaveClass(...buttonClasses('solid', 'compact').slice(-3))
})

test('names the list region Users', async () => {
	renderAt('/users')

	const region = within(await screen.findByRole('region', { name: 'Users' }))
	expect(region.getByRole('searchbox', { name: 'Search users…' })).toBeInTheDocument()
})

test('lines the list up with the title, out to the canvas edges', async () => {
	renderAt('/users')

	const search = await screen.findByRole('searchbox', { name: 'Search users…' })
	expect(search.closest('.godmin-page__list')).not.toBeNull()
})

test('shows an empty state with a solid add button at its default size when no accounts exist', async () => {
	holding([])

	renderAt('/users')

	const title = await screen.findByRole('heading', { level: 3, name: 'No users yet' })
	const add = within(title.closest('.godmin-empty') as HTMLElement).getByRole('link', { name: 'Add User' })
	expect(add).toHaveAttribute('href', '/users/new')
	expect(add).toHaveClass(...buttonClasses('solid').slice(-3))
	expect(add).not.toHaveClass(...compactClasses())
})

test('says no result came back when a search finds no account, and offers no add button', async () => {
	renderAt('/users?search=nobody')

	const title = await screen.findByRole('heading', { level: 3, name: 'No results found' })
	const empty = within(title.closest('.godmin-empty') as HTMLElement)
	expect(empty.getByText("Try adjusting your search or filters to find what you're looking for.")).toBeInTheDocument()
	expect(empty.queryByRole('link')).not.toBeInTheDocument()
	expect(screen.queryByText('No users yet')).not.toBeInTheDocument()
})

test('reads each account status as a badge', async () => {
	holding([signedIn, barred, accountOf('fd', 'Maria Perez', { confirmed: false })])
	renderAt('/users')

	expect((await rowOf(signedIn.name)).getByText('Active')).toBeInTheDocument()
	expect((await rowOf('Ana Lopez')).getByText('Disabled')).toBeInTheDocument()
	expect((await rowOf('Maria Perez')).getByText('Invited')).toBeInTheDocument()
})

test('reads each status in Spanish so it agrees with the user, and offers to activate a disabled one', async () => {
	server.use(graphql.query('AppLocale', () => HttpResponse.json({ data: { locale: 'es-ES' } })))
	await startAppLocale()
	onTestFinished(() => resetLocale())
	holding([signedIn, barred, accountOf('fd', 'Maria Perez', { confirmed: false })])
	renderAt('/users')

	expect((await rowOf(signedIn.name)).getByText('Activo')).toBeInTheDocument()
	expect((await rowOf('Ana Lopez')).getByText('Desactivado')).toBeInTheDocument()
	expect((await rowOf('Maria Perez')).getByText('Invitado')).toBeInTheDocument()
	await userEvent.click((await rowOf('Ana Lopez')).getByRole('button', { name: 'Actions' }))
	expect(await screen.findByRole('menuitem', { name: 'Activar' })).toBeInTheDocument()
})

test('reads the role every account holds by its label, None for one holding no role as WordPress does', async () => {
	holding([signedIn, colleague, accountOf('fc', 'Maria Perez', { role: '' })])
	renderAt('/users')

	expect((await rowOf('Ada Lovelace')).getByText('Member')).toBeInTheDocument()
	expect((await rowOf(signedIn.name)).getByText('Administrator')).toBeInTheDocument()
	expect((await rowOf('Maria Perez')).getByText('None')).toBeInTheDocument()
})

test('reads each core role in Spanish, the way the user reads every other label', async () => {
	server.use(graphql.query('AppLocale', () => HttpResponse.json({ data: { locale: 'es-ES' } })))
	await startAppLocale()
	onTestFinished(() => resetLocale())
	holding([signedIn, colleague, accountOf('fc', 'Maria Perez', { role: '' })])
	renderAt('/users')

	expect((await rowOf('Ada Lovelace')).getByText('Miembro')).toBeInTheDocument()
	expect((await rowOf(signedIn.name)).getByText('Administrador')).toBeInTheDocument()
	expect((await rowOf('Maria Perez')).getByText('Ninguno')).toBeInTheDocument()
})

test('reads a role a plugin adds by its stored name with a capital first letter', async () => {
	holding([signedIn, accountOf('fc', 'Maria Perez', { role: 'provisioner' })])
	renderAt('/users')

	expect((await rowOf('Maria Perez')).getByText('Provisioner')).toBeInTheDocument()
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
	expect((await rowOf(signedIn.name)).getByText('M', { selector: '.godmin-avatar' })).toBeInTheDocument()
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

test('dates every account in the page language once Created is shown', async () => {
	renderAt('/users')
	await rowOf('Ana Lopez')

	await userEvent.click((await viewOptions()).getByRole('button', { name: 'Created' }))

	expect((await rowOf('Ana Lopez')).getByText('Jul 8, 2026 10:00 am')).toBeInTheDocument()
})

test('dates every account in Spanish once Created is shown to a Spanish reader', async () => {
	server.use(graphql.query('AppLocale', () => HttpResponse.json({ data: { locale: 'es-ES' } })))
	await startAppLocale()
	onTestFinished(() => resetLocale())
	renderAt('/users')
	await rowOf('Ana Lopez')

	await userEvent.click((await viewOptions()).getByRole('button', { name: 'Creado' }))

	expect((await rowOf('Ana Lopez')).getByText('8 Jul 2026 10:00')).toBeInTheDocument()
})

test('sorts the accounts newest first when the address names no order, with no Created column', async () => {
	renderAt('/users')

	expect(await shownNames()).toEqual([barred.name, colleague.name, signedIn.name])
	expect(screen.queryByRole('columnheader', { name: /Created/ })).not.toBeInTheDocument()
})

test('offers Created under View options, unticked, and shows it sorted newest first once picked', async () => {
	renderAt('/users')
	await rowOf('Ada Lovelace')
	expect(screen.queryByRole('columnheader', { name: /Created/ })).not.toBeInTheDocument()

	const created = (await viewOptions()).getByRole('button', { name: 'Created' })
	expect(created.querySelector('svg')).toBeNull()
	await userEvent.click(created)

	expect(await screen.findByRole('columnheader', { name: /Created/ })).toHaveAttribute('aria-sort', 'descending')
	expect(await shownNames()).toEqual([barred.name, colleague.name, signedIn.name])
})

test('sorts the accounts by the field and direction the address names', async () => {
	renderAt('/users?sort=email&order=desc')

	expect(await shownNames()).toEqual([barred.name, signedIn.name, colleague.name])
})

test('sorts the accounts by name from A to Z when the address says so', async () => {
	renderAt('/users?sort=name&order=asc')

	expect(await shownNames()).toEqual([colleague.name, barred.name, signedIn.name])
})

test('never sorts by Status or Role, as WordPress sorts only its Username and Email columns', async () => {
	renderAt('/users')
	await screen.findByRole('table')

	for (const name of ['Status', 'Role']) {
		await userEvent.click(screen.getByRole('button', { name }))
		expect(await screen.findByRole('menuitem', { name: 'Hide column' })).toBeInTheDocument()
		expect(screen.queryByRole('menuitemradio', { name: 'Sort ascending' })).not.toBeInTheDocument()
		await userEvent.keyboard('{Escape}')
	}
})

test('searches the accounts by name', async () => {
	renderAt('/users')

	await userEvent.type(await screen.findByRole('searchbox', { name: 'Search users…' }), 'Lopez')

	await waitFor(async () => expect(await shownNames()).toEqual(['Ana Lopez']))
})

test('searches the accounts by email', async () => {
	renderAt('/users')

	await userEvent.type(await screen.findByRole('searchbox', { name: 'Search users…' }), 'admin@')

	await waitFor(async () => expect(await shownNames()).toEqual([signedIn.name]))
})

test('narrows the accounts to the statuses the address names', async () => {
	const filters = encodeURIComponent(JSON.stringify([{ field: 'status', operator: 'isAny', value: ['disabled'] }]))
	renderAt(`/users?filters=${filters}`)

	expect(await shownNames()).toEqual(['Ana Lopez'])
})

test('narrows the accounts to the roles the address names', async () => {
	const filters = encodeURIComponent(JSON.stringify([{ field: 'role', operator: 'isAny', value: ['admin'] }]))
	renderAt(`/users?filters=${filters}`)

	expect(await shownNames()).toEqual([signedIn.name])
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
	expect(offered).toEqual(['Administrator', 'Member', 'No role'])
})

test('offers as statuses only the ones the accounts hold, as WordPress lists only the roles in use', async () => {
	renderAt('/users?filters=' + encodeURIComponent(JSON.stringify([{ field: 'status', operator: 'isAny', value: [] }])))

	await userEvent.click(await screen.findByRole('button', { name: 'Filter' }))
	await userEvent.click(await screen.findByRole('button', { name: 'Status', pressed: false }))

	const offered = (await screen.findAllByRole('option')).map((option) => option.textContent)
	expect(offered).toEqual(['Active', 'Disabled'])
})

test('pages the accounts by the page size the admin settings name', async () => {
	paging([2, 4], 2)
	renderAt('/users')

	expect(await shownNames()).toEqual(['Ana Lopez', 'Ada Lovelace'])
	await userEvent.click(screen.getByRole('button', { name: 'Next page' }))

	await waitFor(async () => expect(await shownNames()).toEqual([signedIn.name]))
})

test('pages by the size the admin settings name when the address names one they do not offer', async () => {
	paging([2, 4], 2)
	renderAt('/users?perPage=3')

	expect(await shownNames()).toEqual(['Ana Lopez', 'Ada Lovelace'])
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
						toastActionMilliseconds: 10000,
						dialogWriteTimeoutMilliseconds: 60000,
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

	await waitFor(async () => expect(await shownNames()).toEqual([signedIn.name]), { timeout: 3000 })
})

test('shows every account on one page when the page size could not be read', async () => {
	server.use(
		graphql.query('AdminSettings', () =>
			HttpResponse.json({ data: null, errors: [{ message: 'internal error' }] }),
		),
	)
	renderAt('/users')

	await waitFor(async () => expect(await shownNames()).toEqual([barred.name, colleague.name, signedIn.name]))
	expect(screen.queryByRole('button', { name: 'Next page' })).not.toBeInTheDocument()
})

test('counts the accounts, because the list offers bulk actions', async () => {
	renderAt('/users')

	expect(await screen.findByText('3 items')).toBeInTheDocument()
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
	await userEvent.type(await screen.findByLabelText('Email (Required)'), 'ada@example.com')
	await userEvent.type(screen.getByLabelText('Name (Required)'), 'Ada Lovelace')
	expect(screen.queryByLabelText('Password')).not.toBeInTheDocument()
	await userEvent.click(screen.getByRole('button', { name: 'Send invitation' }))
}

/**
 * Returns the breadcrumb that leads to the user list.
 * @returns The link.
 */
async function usersBreadcrumb() {
	const breadcrumbs = await screen.findByRole('navigation', { name: 'Breadcrumbs' })
	return within(breadcrumbs).getByRole('link', { name: 'Users' })
}

test('leads the breadcrumb of the new user form back to the user list', async () => {
	const { router } = renderRoutedAt('/users/new')
	const users = await usersBreadcrumb()

	expect(users).toHaveAttribute('href', '/users')
	expect(users).not.toHaveAttribute('aria-current')
	await userEvent.click(users)

	await waitFor(() => expect(router.state.location.pathname).toBe('/users'))
})

/** activationLink is the link an undelivered invitation hands over. */
const activationLink = '/activate?token=t-4'

/** Answers every invitation as undelivered, with the activation link to hand over. */
function serveUndeliveredInvite() {
	server.use(
		graphql.mutation('Invite', () =>
			HttpResponse.json({
				data: { invite: { __typename: 'InvitePayload', delivered: false, activationLink } },
			}),
		),
	)
}

test('keeps the breadcrumb beside the activation link', async () => {
	serveUndeliveredInvite()
	renderAt('/users/new')
	await submitNewUser()
	await screen.findByLabelText('Activation link')

	expect(await usersBreadcrumb()).toHaveAttribute('href', '/users')
})

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
	serveUndeliveredInvite()
	renderAt('/users/new')
	await submitNewUser()
	await screen.findByLabelText('Activation link')

	await userEvent.click(screen.getByRole('button', { name: 'Done' }))

	expect(await screen.findByRole('heading', { name: 'Users' })).toBeInTheDocument()
	expect(screen.queryByText('Invitation sent.')).not.toBeInTheDocument()
})

test('shows the activation link when no mail server delivered the invitation', async () => {
	serveUndeliveredInvite()
	renderAt('/users/new')

	await submitNewUser()

	expect(await screen.findByLabelText('Activation link')).toHaveValue(activationLink)
})

test('says in a toast what the server said when it names no reason', async () => {
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

	expect(
		await screen.findByText('Could not send the invitation. that name is too long', { selector: '.godmin-toast' }),
	).toBeInTheDocument()
	expect(screen.queryByRole('alert')).not.toBeInTheDocument()
	expect(screen.getByLabelText('Name (Required)')).toBeValid()
})

test('says in a toast that the invitation could not be sent when the failure is about no field', async () => {
	server.use(
		graphql.mutation('Invite', () =>
			HttpResponse.json({ data: null, errors: [{ message: 'internal error' }] }),
		),
	)
	renderAt('/users/new')

	await submitNewUser()

	expect(await screen.findByText('Could not send the invitation.', { selector: '.godmin-toast' })).toBeInTheDocument()
	expect(screen.queryByText('internal error')).not.toBeInTheDocument()
	expect(screen.queryByRole('alert')).not.toBeInTheDocument()
	expect(screen.getByLabelText('Email (Required)')).toHaveValue('ada@example.com')
	expect(screen.getByLabelText('Name (Required)')).toHaveValue('Ada Lovelace')
	await waitFor(() =>
		expect(screen.getByRole('button', { name: 'Send invitation' })).not.toHaveAttribute('aria-disabled', 'true'),
	)
})

test('says in a toast that the invitation failed once the reader left the form for the list', async () => {
	const answer = gate()
	server.use(
		graphql.mutation('Invite', async () => {
			await answer.held
			return HttpResponse.json({
				data: null,
				errors: [{ message: 'name too long', extensions: { code: 'VALIDATION', reason: 'name_too_long' } }],
			})
		}),
	)
	renderAt('/users/new')

	await submitNewUser()
	await userEvent.click(await usersBreadcrumb())
	await screen.findByRole('heading', { level: 2, name: 'Users' })
	answer.release()

	expect(
		await screen.findByText('Could not send the invitation. This name is too long.', { selector: '.godmin-toast' }),
	).toBeInTheDocument()
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
