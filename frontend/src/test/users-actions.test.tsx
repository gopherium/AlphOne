// SPDX-License-Identifier: AGPL-3.0-or-later

import { rememberFormatLocale } from '@alphone/frontend-sdk'
import { HttpResponse, graphql, http, server } from '@alphone/frontend-sdk/testing'
import { setViewport } from '@gopherium/godmin/testing'
import { resetLocale } from '@gopherium/gottext/testing'
import { screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { beforeAll, beforeEach, expect, onTestFinished, test } from 'vitest'

import { configureAppErrorText } from '../i18n/errors'
import { startAppLocale } from '../i18n/start'
import { gate } from './gate'
import { busyClasses, renderAt } from './render'
import { accountOf, barred, colleague, rowOf, serveUsers, signedIn } from './usersScreen'
import type { account } from './usersScreen'

/** held is the account list the users query answers, changed by the mutations. */
let held: account[] = []

/**
 * Serves the given accounts from the users query.
 * @param accounts - The accounts the list holds.
 */
function holding(accounts: account[]) {
	held = accounts
	serveUsers(() => held)
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

test('offers no row action on the signed-in account', async () => {
	renderAt('/users')

	await rowOf('Ada Lovelace')
	expect((await rowOf(signedIn.name)).queryByRole('button', { name: 'Actions' })).not.toBeInTheDocument()
	expect((await rowOf(signedIn.name)).getByRole('checkbox')).toHaveAttribute('aria-disabled', 'true')
})

test('offers no enable on the signed-in account, even while it reads disabled', async () => {
	holding([{ ...signedIn, disabled: true }, colleague])
	renderAt('/users')

	await rowOf('Ada Lovelace')
	const own = await rowOf(signedIn.name)
	expect(own.getByText('Disabled')).toBeInTheDocument()
	expect(own.queryByRole('button', { name: 'Actions' })).not.toBeInTheDocument()
	expect(own.queryByRole('button', { name: 'Enable' })).not.toBeInTheDocument()
})

test('offers no resend on the signed-in account, even while it reads invited', async () => {
	holding([{ ...signedIn, confirmed: false }, colleague])
	renderAt('/users')

	await rowOf('Ada Lovelace')
	const own = await rowOf(signedIn.name)
	expect(own.getByText('Invited')).toBeInTheDocument()
	expect(own.queryByRole('button', { name: 'Actions' })).not.toBeInTheDocument()
})

test('draws Enable on a disabled row as its primary action, and never Disable', async () => {
	renderAt('/users')

	expect((await rowOf('Ana Lopez')).getByRole('button', { name: 'Enable' })).toBeInTheDocument()
	const active = await rowOf('Ada Lovelace')
	expect(active.queryByRole('button', { name: 'Enable' })).not.toBeInTheDocument()
	expect(active.queryByRole('button', { name: 'Disable' })).not.toBeInTheDocument()
})

/**
 * Returns the labels of the actions the Actions menu of one row lists, closing the menu again.
 * @param row - The row queries.
 * @returns The action labels.
 */
async function menuOf(row: ReturnType<typeof within>) {
	await userEvent.click(row.getByRole('button', { name: 'Actions' }))
	const labels = (await screen.findAllByRole('menuitem')).map((item) => item.textContent)
	await userEvent.keyboard('{Escape}')
	await waitFor(() => expect(screen.queryByRole('menuitem')).not.toBeInTheDocument())
	return labels
}

test('offers no role change, resend, disable or enable on an account beyond the reach of the reader', async () => {
	holding([
		signedIn,
		accountOf('fd', 'Maria Perez'),
		accountOf('fb', 'Higher Role Account', { role: 'steward', withinReach: false }),
		accountOf('fa', 'Disabled Higher Role Account', { role: 'steward', disabled: true, withinReach: false }),
		accountOf('f9', 'Invited Higher Role Account', { role: 'steward', confirmed: false, withinReach: false }),
	])
	renderAt('/users')

	expect(await menuOf(await rowOf('Maria Perez'))).toEqual(['Change role', 'Disable'])
	for (const name of ['Higher Role Account', 'Disabled Higher Role Account', 'Invited Higher Role Account']) {
		const row = await rowOf(name)
		expect(row.queryByRole('button', { name: 'Actions' })).not.toBeInTheDocument()
		expect(row.queryByRole('button', { name: 'Enable' })).not.toBeInTheDocument()
		expect(row.getByRole('checkbox')).toHaveAttribute('aria-disabled', 'true')
	}
})

test('offers Enable only on a disabled account whose role the reader may give', async () => {
	holding([
		signedIn,
		accountOf('fd', 'Maria Perez', { disabled: true }),
		accountOf('fc', 'No Role Account', { disabled: true, role: '' }),
		accountOf('fb', 'Lost Role Account', { disabled: true, role: 'auditor' }),
	])
	renderAt('/users')

	expect((await rowOf('Maria Perez')).getByRole('button', { name: 'Enable' })).toBeInTheDocument()
	for (const name of ['No Role Account', 'Lost Role Account']) {
		const row = await rowOf(name)
		expect(row.queryByRole('button', { name: 'Enable' })).not.toBeInTheDocument()
		expect(await menuOf(row)).toEqual(['Change role'])
	}
})

test('lays the accounts out as a list on a phone', async () => {
	setViewport({ matches: true })
	renderAt('/users')

	expect(await screen.findByText('Ada Lovelace')).toBeInTheDocument()
	expect(screen.queryByRole('table')).not.toBeInTheDocument()
})

test('opens Change role from a tap on a phone row the reader may change, leaving the row unselected', async () => {
	setViewport({ matches: true })
	renderAt('/users')

	const item = await screen.findByRole('button', { name: 'Ada Lovelace' })
	await userEvent.click(item)

	expect((await roleDialog()).getByText('Choose the role of Ada Lovelace.')).toBeInTheDocument()
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

test('disables an account from its row actions and names it in a toast', async () => {
	const writes = writesDisabled()
	renderAt('/users')

	await act('Ada Lovelace', 'Disable')

	expect(await screen.findByText('"Ada Lovelace" disabled.')).toBeInTheDocument()
	expect(writes).toEqual([{ id: colleague.id, disabled: true }])
	await waitFor(async () => expect((await rowOf('Ada Lovelace')).getByText('Disabled')).toBeInTheDocument())
})

test('enables a disabled account from its row actions and names it in a toast', async () => {
	const writes = writesDisabled()
	renderAt('/users')

	await act('Ana Lopez', 'Enable')

	expect(await screen.findByText('"Ana Lopez" enabled.')).toBeInTheDocument()
	expect(writes).toEqual([{ id: barred.id, disabled: false }])
})

test('enables a disabled account from the Enable its row draws', async () => {
	const writes = writesDisabled()
	renderAt('/users')

	await userEvent.click((await rowOf('Ana Lopez')).getByRole('button', { name: 'Enable' }))

	expect(await screen.findByText('"Ana Lopez" enabled.')).toBeInTheDocument()
	expect(writes).toEqual([{ id: barred.id, disabled: false }])
})

test('puts the focus on the actions of the row once its pressed Enable went with the reload', async () => {
	writesDisabled()
	renderAt('/users')

	await userEvent.click((await rowOf('Ana Lopez')).getByRole('button', { name: 'Enable' }))

	await waitFor(async () => expect((await rowOf('Ana Lopez')).getByRole('button', { name: 'Actions' })).toHaveFocus())
	expect((await rowOf('Ana Lopez')).queryByRole('button', { name: 'Enable' })).not.toBeInTheDocument()
})

test('shows a long account name whole in the toast', async () => {
	holding([signedIn, accountOf('fd', 'Maria Perez of the accounting and payroll office team')])
	writesDisabled()
	renderAt('/users')

	await act('Maria Perez of the accounting and payroll office team', 'Disable')

	expect(
		await screen.findByText('"Maria Perez of the accounting and payroll office team" disabled.'),
	).toBeInTheDocument()
})

test('names a disabled account in a Spanish toast', async () => {
	server.use(graphql.query('AppLocale', () => HttpResponse.json({ data: { locale: 'es-ES' } })))
	await startAppLocale()
	onTestFinished(() => resetLocale())
	writesDisabled()
	renderAt('/users')

	await act('Ada Lovelace', 'Desactivar')

	expect(await screen.findByText('«Ada Lovelace» se ha desactivado.')).toBeInTheDocument()
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

	expect(
		await screen.findByText('Somebody must be able to manage users.', { selector: '.godmin-toast' }),
	).toBeInTheDocument()
	expect(screen.queryByRole('alert')).not.toBeInTheDocument()
	expect(screen.queryByText(/cannot be disabled$/)).not.toBeInTheDocument()
	expect(screen.queryByText('"Ada Lovelace" disabled.')).not.toBeInTheDocument()
})

test('says plainly that an account could not be enabled when the server names no reason', async () => {
	server.use(
		graphql.mutation('SetUserDisabled', () =>
			HttpResponse.json({ data: null, errors: [{ message: 'internal error' }] }),
		),
	)
	renderAt('/users')

	await act('Ana Lopez', 'Enable')

	expect(await screen.findByText('The user could not be enabled.', { selector: '.godmin-toast' })).toBeInTheDocument()
	expect(screen.queryByRole('alert')).not.toBeInTheDocument()
	expect(screen.queryByText('internal error')).not.toBeInTheDocument()
})

test('says why an enable was refused when the roles the reader may give went stale', async () => {
	configureAppErrorText()
	holding([signedIn, accountOf('fd', 'Maria Perez', { disabled: true })])
	server.use(
		graphql.mutation('SetUserDisabled', () =>
			HttpResponse.json({
				data: null,
				errors: [
					{
						message: 'you are not allowed to give users that role',
						extensions: { code: 'VALIDATION', reason: 'role_not_grantable' },
					},
				],
			}),
		),
	)
	renderAt('/users')

	await act('Maria Perez', 'Enable')

	expect(
		await screen.findByText('Sorry, you are not allowed to give users that role.', { selector: '.godmin-toast' }),
	).toBeInTheDocument()
	expect(screen.queryByRole('alert')).not.toBeInTheDocument()
	expect(screen.queryByText('"Maria Perez" enabled.')).not.toBeInTheDocument()
})

test('says in a toast that a disable from another site was refused, in the words AlphOne gives it', async () => {
	configureAppErrorText()
	server.use(
		http.post('/api/graphql', async ({ request }) => {
			const { query } = (await request.clone().json()) as { query: string }
			return query.includes('SetUserDisabled')
				? HttpResponse.json({ error: 'cross-origin request refused', code: 'request_cross_origin' }, { status: 403 })
				: undefined
		}),
	)
	renderAt('/users')

	await act('Ada Lovelace', 'Disable')

	expect(
		await screen.findByText(
			'This request came from a page on another site, so it was refused. Open the admin on this site and try again.',
			{ selector: '.godmin-toast' },
		),
	).toBeInTheDocument()
	expect(screen.queryByText('cross-origin request refused')).not.toBeInTheDocument()
})

test('says in WordPress words that the reader may no longer edit users when its role lost the capability', async () => {
	configureAppErrorText()
	server.use(
		graphql.mutation('SetUserDisabled', () =>
			HttpResponse.json({
				data: null,
				errors: [
					{
						message: 'admin required',
						extensions: {
							code: 'UNAUTHORIZED',
							reason: 'capability_missing',
							meta: { scope: 'users:write', capability: 'manage_users' },
						},
					},
				],
			}),
		),
	)
	renderAt('/users')

	await act('Ada Lovelace', 'Disable')

	expect(
		await screen.findByText('Sorry, you are not allowed to edit users.', { selector: '.godmin-toast' }),
	).toBeInTheDocument()
	expect(screen.queryByText(/manage_users/)).not.toBeInTheDocument()
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

/**
 * Returns the buttons of the bar that acts on the selected accounts.
 * @returns The bar queries.
 */
async function bulkBar() {
	return within(
		await waitFor(() => {
			const bar = document.querySelector<HTMLElement>('.dataviews-bulk-actions__actions')
			expect(bar).not.toBeNull()
			return bar as HTMLElement
		}),
	)
}

test('disables every selected account at once with one toast counting them in the format locale', async () => {
	rememberFormatLocale('es-ES-u-nu-deva')
	holding([signedIn, colleague, accountOf('fd', 'Maria Perez')])
	const writes = writesDisabled()
	renderAt('/users')

	await select('Ada Lovelace', 'Maria Perez')
	await userEvent.click(await (await bulkBar()).findByRole('button', { name: 'Disable' }))

	expect(await screen.findByText('२ users disabled.')).toBeInTheDocument()
	expect(writes.map((write) => write.disabled)).toEqual([true, true])
})

test('offers Enable before Disable in the bulk bar, the one that locks accounts out last', async () => {
	renderAt('/users')

	await select('Ada Lovelace', 'Ana Lopez')

	const bar = await bulkBar()
	const enable = await bar.findByRole('button', { name: 'Enable' })
	const disable = bar.getByRole('button', { name: 'Disable' })
	expect(enable.compareDocumentPosition(disable) & Node.DOCUMENT_POSITION_FOLLOWING).toBeTruthy()
})

test('counts only the accounts a bulk disable could not change, in one toast', async () => {
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
	await userEvent.click(await (await bulkBar()).findByRole('button', { name: 'Disable' }))

	expect(await screen.findByText('1 user could not be disabled.', { selector: '.godmin-toast' })).toBeInTheDocument()
	expect(screen.queryByText('1 user disabled.')).not.toBeInTheDocument()
	expect(document.querySelectorAll('.godmin-toast')).toHaveLength(1)
	expect(screen.queryByRole('alert')).not.toBeInTheDocument()
})

test('enables every selected account at once with one toast', async () => {
	holding([signedIn, barred, accountOf('fd', 'Maria Perez', { disabled: true })])
	writesDisabled()
	renderAt('/users')

	await select('Ana Lopez', 'Maria Perez')
	await userEvent.click(await (await bulkBar()).findByRole('button', { name: 'Enable' }))

	expect(await screen.findByText('2 users enabled.')).toBeInTheDocument()
})

test('counts the accounts a bulk enable could not change in one toast, in the format locale', async () => {
	rememberFormatLocale('es-ES-u-nu-deva')
	holding([signedIn, barred, accountOf('fd', 'Maria Perez', { disabled: true })])
	server.use(
		graphql.mutation('SetUserDisabled', () =>
			HttpResponse.json({ data: null, errors: [{ message: 'internal error' }] }),
		),
	)
	renderAt('/users')

	await select('Ana Lopez', 'Maria Perez')
	await userEvent.click(await (await bulkBar()).findByRole('button', { name: 'Enable' }))

	expect(await screen.findByText('२ users could not be enabled.', { selector: '.godmin-toast' })).toBeInTheDocument()
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

/**
 * Returns the open Change role dialog.
 * @returns The dialog queries.
 */
async function roleDialog() {
	return within(await screen.findByRole('dialog', { name: 'Change role' }))
}

/** Presses Cancel in the open Change role dialog and waits for it to leave. */
async function closeRoleDialog() {
	await userEvent.click((await roleDialog()).getByRole('button', { name: 'Cancel' }))
	await waitFor(() => expect(screen.queryByRole('dialog')).not.toBeInTheDocument())
}

/** Picks Administrator in the open Change role dialog and sends it. */
async function changeToAdministrator() {
	await userEvent.click((await roleDialog()).getByRole('combobox', { name: 'Role' }))
	await userEvent.click(await screen.findByRole('option', { name: 'Administrator' }))
	await userEvent.click((await roleDialog()).getByRole('button', { name: 'Change role' }))
}

test('opens Change role from a click on the name of an account the reader may change', async () => {
	renderAt('/users')

	await userEvent.click((await rowOf(colleague.name)).getByText(colleague.name))

	expect((await roleDialog()).getByText('Choose the role of Ada Lovelace.')).toBeInTheDocument()
})

test('changes the role of every selected account from the bulk bar with one toast', async () => {
	holding([signedIn, colleague, accountOf('fd', 'Maria Perez')])
	const asked = writesRole()
	renderAt('/users')

	await select('Ada Lovelace', 'Maria Perez')
	await userEvent.click(await (await bulkBar()).findByRole('button', { name: 'Change role' }))
	expect((await roleDialog()).getByText('Choose the role of 2 users.')).toBeInTheDocument()
	await changeToAdministrator()

	expect(await screen.findByText('Changed roles.', { selector: '.godmin-toast' })).toBeInTheDocument()
	expect(asked).toEqual(['admin', 'admin'])
	expect(document.querySelectorAll('.godmin-toast')).toHaveLength(1)
})

test('changes the role of another account in a dialog and names the account in a toast', async () => {
	const asked = writesRole()
	renderAt('/users')

	await act('Ada Lovelace', 'Change role')
	await changeToAdministrator()

	expect(await screen.findByText('Role changed for "Ada Lovelace".')).toBeInTheDocument()
	expect(asked).toEqual(['admin'])
	await waitFor(() => expect(screen.queryByRole('dialog')).not.toBeInTheDocument())
	await waitFor(async () => expect((await rowOf('Ada Lovelace')).getByText('Administrator')).toBeInTheDocument())
})

test('keeps Change role open with a spinner while it writes, then closes and toasts before the reload', async () => {
	renderAt('/users')
	await rowOf('Ada Lovelace')
	const write = gate()
	const reload = gate()
	server.use(
		graphql.mutation('SetUserRole', async ({ variables }) => {
			await write.held
			held = held.map((user) => (user.id === String(variables.id) ? { ...user, role: String(variables.role) } : user))
			return HttpResponse.json({ data: { setUserRole: true } })
		}),
		graphql.query('Users', async () => {
			await reload.held
			return HttpResponse.json({ data: { users: held } })
		}),
	)

	await act('Ada Lovelace', 'Change role')
	await changeToAdministrator()

	const send = (await roleDialog()).getByRole('button', { name: 'Change role' })
	await waitFor(() => expect(send).toHaveAttribute('aria-disabled', 'true'))
	expect([...send.classList]).toEqual(expect.arrayContaining(busyClasses()))
	expect(document.querySelector('.godmin-toast')).toBeNull()
	write.release()
	await waitFor(() => expect(screen.queryByRole('dialog')).not.toBeInTheDocument())
	expect(await screen.findByText('Role changed for "Ada Lovelace".')).toBeInTheDocument()
	await waitFor(() => expect(screen.getByRole('table')).toHaveAttribute('aria-busy', 'true'))
	expect((await rowOf('Ada Lovelace')).getByText('Member')).toBeInTheDocument()
	reload.release()
	await waitFor(() => expect(screen.getByRole('table')).not.toHaveAttribute('aria-busy', 'true'))
	expect((await rowOf('Ada Lovelace')).getByText('Administrator')).toBeInTheDocument()
})

test('gives the focus back to the actions of the row once a role change reloaded the list', async () => {
	writesRole()
	renderAt('/users')

	await act('Ada Lovelace', 'Change role')
	await changeToAdministrator()

	await screen.findByText('Role changed for "Ada Lovelace".')
	await waitFor(() => expect(screen.getByRole('table')).not.toHaveAttribute('aria-busy', 'true'))
	const row = await rowOf('Ada Lovelace')
	await waitFor(() => expect(row.getByRole('button', { name: 'Actions' })).toHaveFocus())
})

test('says inside Change role that the change failed, marks no reload, and opens again on the role held', async () => {
	renderAt('/users')
	await rowOf('Ada Lovelace')
	const reload = gate()
	let reads = 0
	server.use(
		graphql.mutation('SetUserRole', ({ variables }) => {
			held = held.map((user) => (user.id === String(variables.id) ? { ...user, email: 'reloaded@example.com' } : user))
			return HttpResponse.json({ data: null, errors: [{ message: 'internal error' }] })
		}),
		graphql.query('Users', async () => {
			reads += 1
			await reload.held
			return HttpResponse.json({ data: { users: held } })
		}),
	)

	await act('Ada Lovelace', 'Change role')
	await changeToAdministrator()

	const dialog = await roleDialog()
	expect(await dialog.findByText('The role could not be changed.')).toBeInTheDocument()
	expect(document.querySelector('.godmin-toast')).toBeNull()
	await waitFor(() => expect(reads).toBe(1))
	expect(screen.getByRole('table', { hidden: true })).not.toHaveAttribute('aria-busy', 'true')
	reload.release()
	expect(await screen.findByText('reloaded@example.com')).toBeInTheDocument()
	await closeRoleDialog()
	await act('Ada Lovelace', 'Change role')
	expect((await roleDialog()).getByRole('combobox', { name: 'Role' })).toHaveTextContent('Member')
})

test('offers only the roles the reader may grant, the one with the fewest capabilities first', async () => {
	renderAt('/users')

	await act('Ada Lovelace', 'Change role')
	await userEvent.click(await screen.findByRole('combobox', { name: 'Role' }))

	const offered = (await screen.findAllByRole('option')).map((option) => option.textContent)
	expect(offered).toEqual(['Member', 'Administrator'])
})

test('closes the role modal without writing when the reader cancels', async () => {
	const asked = writesRole()
	renderAt('/users')

	await act('Ada Lovelace', 'Change role')
	await userEvent.click(await screen.findByRole('button', { name: 'Cancel' }))

	await waitFor(() => expect(screen.queryByRole('dialog')).not.toBeInTheDocument())
	expect(asked).toEqual([])
})

test.each([
	{ reason: 'user_not_editable', code: 'VALIDATION', words: 'Sorry, you are not allowed to edit this user.' },
	{ reason: 'role_not_grantable', code: 'VALIDATION', words: 'Sorry, you are not allowed to give users that role.' },
	{ reason: 'role_unknown', code: 'VALIDATION', words: 'Sorry, you are not allowed to give users that role.' },
	{
		reason: 'role_changed',
		code: 'CONFLICT',
		words: 'The role of that account just changed. Reload the page and try again.',
	},
	{
		reason: 'tenant_deactivated',
		code: 'UNAUTHORIZED',
		words: 'This workspace has been deactivated. Contact your operator.',
	},
])('says inside Change role why the change was rejected for $reason', async ({ reason, code, words }) => {
	configureAppErrorText()
	server.use(
		graphql.mutation('SetUserRole', () =>
			HttpResponse.json({ data: null, errors: [{ message: 'role write refused', extensions: { code, reason } }] }),
		),
	)
	renderAt('/users')

	await act('Ada Lovelace', 'Change role')
	await changeToAdministrator()

	expect(await (await roleDialog()).findByText(words)).toBeInTheDocument()
	expect(screen.getByRole('dialog', { name: 'Change role' })).toBeInTheDocument()
	expect(document.querySelector('.godmin-toast')).toBeNull()
	expect(screen.queryByText('role write refused')).not.toBeInTheDocument()
	expect(screen.queryByText(/Role changed for/)).not.toBeInTheDocument()
})

test('reads the accounts again after a rejected role change, then opens again on the role now held', async () => {
	configureAppErrorText()
	holding([signedIn, accountOf('fd', 'Maria Perez')])
	server.use(
		graphql.mutation('SetUserRole', ({ variables }) => {
			held = held.map((user) => (user.id === String(variables.id) ? { ...user, role: 'admin' } : user))
			return HttpResponse.json({
				data: null,
				errors: [{ message: 'role changed', extensions: { code: 'CONFLICT', reason: 'role_changed' } }],
			})
		}),
	)
	renderAt('/users')

	await act('Maria Perez', 'Change role')
	await changeToAdministrator()

	expect(
		await (await roleDialog()).findByText('The role of that account just changed. Reload the page and try again.'),
	).toBeInTheDocument()
	await closeRoleDialog()
	await waitFor(async () => expect((await rowOf('Maria Perez')).getByText('Administrator')).toBeInTheDocument())
	await act('Maria Perez', 'Change role')
	expect((await roleDialog()).getByRole('combobox', { name: 'Role' })).toHaveTextContent('Administrator')
})

test('says plainly inside Change role that a role could not be changed when the server names no reason', async () => {
	server.use(
		graphql.mutation('SetUserRole', () =>
			HttpResponse.json({ data: null, errors: [{ message: 'internal error' }] }),
		),
	)
	renderAt('/users')

	await act('Ada Lovelace', 'Change role')
	await changeToAdministrator()

	expect(await (await roleDialog()).findByText('The role could not be changed.')).toBeInTheDocument()
	expect(document.querySelector('.godmin-toast')).toBeNull()
	expect(screen.queryByText('internal error')).not.toBeInTheDocument()
})
