// SPDX-License-Identifier: AGPL-3.0-or-later

import { HttpResponse, delay, graphql, server } from '@alphone/frontend-sdk/testing'
import { setViewport } from '@gopherium/godmin/testing'
import { resetLocale } from '@gopherium/gottext/testing'
import { sessionQueryKey } from '@gopherium/react-auth'
import { screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { beforeEach, expect, onTestFinished, test, vi } from 'vitest'

import { configureAppErrorText } from '../i18n/errors'
import { startAppLocale } from '../i18n/start'
import { createAppRouter } from '../router'
import { badgeClasses, buttonClasses, renderAt } from './render'

/** identity is one way a contact is reached, as the contact page answers it. */
interface identity {
	__typename: 'ContactIdentity'
	id: string
	channel: string
	identifier: string
}

/** contact is one contact as the contact page answers it. */
interface contact {
	__typename: 'Contact'
	id: string
	name: string
	createdAt: string
	identities: identity[]
}

/**
 * Builds one contact the contact page answers.
 * @param id - The last two hex digits of its identifier.
 * @param name - Its name.
 * @param createdAt - When it was created.
 * @param reached - The channel and identifier of each of its identities.
 * @returns The contact.
 */
function contactOf(id: string, name: string, createdAt: string, reached: [string, string][] = []): contact {
	return {
		__typename: 'Contact',
		id: `0198c000-0000-7000-8000-0000000000${id}`,
		name,
		createdAt,
		identities: reached.map(([channel, identifier], index) => ({
			__typename: 'ContactIdentity',
			id: `0198c000-0000-7000-8000-00000000${id}0${index}`,
			channel,
			identifier,
		})),
	}
}

const ana = contactOf('01', 'Ana Lopez', '2026-07-08T10:00:00Z', [
	['email', 'ana@example.com'],
	['phone', '184467235'],
])
const ada = contactOf('02', 'Ada Lovelace', '2026-07-07T10:00:00Z', [
	['phone', '184467236'],
	['whatsapp', '184467236'],
	['whatsapp', '184467237'],
])
const maria = contactOf('03', 'Maria Perez', '2026-07-06T10:00:00Z')

/** settled is how long a wait lasts for the rows. */
const settled = { timeout: 3000 }

/** held is the directory the contact page answers from, changed by the mutations. */
let held: contact[] = []

/** asked records the variables of every contact page the screen asked for. */
let asked: Record<string, unknown>[] = []

/**
 * Returns the contacts one page request matches, as the server would.
 * @param variables - The variables the request carried.
 * @returns The matching contacts, in the order the request names.
 */
function matching(variables: Record<string, unknown>): contact[] {
	const q = String(variables.q ?? '').toLowerCase()
	const channels = (variables.channels as string[] | null) ?? []
	const sorted = held
		.filter(
			(row) =>
				row.name.toLowerCase().includes(q) || row.identities.some((each) => each.identifier.includes(q)),
		)
		.filter((row) => channels.length === 0 || row.identities.some((each) => channels.includes(each.channel)))
		.sort((a, b) =>
			variables.orderBy === 'NAME' ? a.name.localeCompare(b.name) : a.createdAt.localeCompare(b.createdAt),
		)
	return variables.order === 'DESC' ? sorted.reverse() : sorted
}

/**
 * Serves the given contacts from the contact page, paged as the request names.
 * @param contacts - The contacts the directory holds.
 * @param byDefault - The rows a page holds when the request names no limit.
 */
function serving(contacts: contact[], byDefault = 50) {
	held = contacts
	server.use(
		graphql.query('ContactPage', ({ variables }) => {
			asked.push(variables)
			const found = matching(variables)
			const limit = (variables.limit as number | null) ?? byDefault
			const offset = (variables.offset as number | null) ?? 0
			return HttpResponse.json({
				data: {
					contactPage: {
						__typename: 'ContactPage',
						items: found.slice(offset, offset + limit),
						total: found.length,
						limit,
					},
				},
			})
		}),
	)
}

/**
 * Serves the admin settings with the given page sizes and contact page cap.
 * @param sizes - The page sizes a list offers.
 * @param size - The page size a list opens on.
 * @param cap - The most contacts one page holds.
 */
function paging(sizes: number[], size: number, cap = 200) {
	server.use(
		graphql.query('AdminSettings', () =>
			HttpResponse.json({
				data: {
					adminSettings: {
						__typename: 'AdminSettings',
						toastMilliseconds: 6000,
						listPageSizes: sizes,
						listPageSize: size,
						contactPageCap: cap,
						formatLocale: 'es-ES',
					},
				},
			}),
		),
	)
}

/**
 * Serves the detail of every contact the directory holds.
 */
function detailing() {
	server.use(
		graphql.query('ContactDetail', ({ variables }) => {
			const found = held.find((row) => row.id === variables.id) as contact
			return HttpResponse.json({
				data: {
					contact: {
						...found,
						identities: found.identities.map((each) => ({ ...each, displayName: '' })),
						tasks: {
							__typename: 'TaskConnection',
							edges: [],
							pageInfo: { __typename: 'PageInfo', hasNextPage: false, endCursor: null },
						},
					},
				},
			})
		}),
	)
}

/**
 * Returns the table row holding the given text.
 * @param text - A name or identifier the row shows.
 * @returns The row queries.
 */
async function rowOf(text: string) {
	return within(await screen.findByRole('row', { name: new RegExp(text) }, settled))
}

/**
 * Returns the names the list shows, in order.
 * @returns The names.
 */
async function shownNames() {
	await screen.findByRole('table', {}, settled)
	return screen
		.getAllByRole('row')
		.slice(1)
		.map((row) => held.find((each) => row.textContent?.includes(each.name))?.name)
}

/**
 * Opens the row actions of the named contact and picks one.
 * @param name - The contact the row shows.
 * @param action - The label of the action to pick.
 */
async function act(name: string, action: string) {
	const row = await rowOf(name)
	await userEvent.click(row.getByRole('button', { name: 'Actions' }))
	await userEvent.click(await screen.findByRole('menuitem', { name: action }))
}

beforeEach(async () => {
	asked = []
	serving([ana, ada, maria])
	detailing()
	await import('../contacts/ContactsScreen')
})

test('keeps in the address only what a list view can hold', () => {
	const { validateSearch } = createAppRouter().routesById['/contacts'].options

	const kept = (validateSearch as (raw: Record<string, unknown>) => unknown)({
		search: 'ana',
		page: 'abc',
		order: 'up',
		tab: 'other',
	})

	expect(kept).toEqual({ search: 'ana' })
})

test('loads the contacts list in a chunk of its own, off the first paint', () => {
	const { component } = createAppRouter().routesById['/contacts'].options

	expect(typeof (component as { preload?: unknown }).preload).toBe('function')
})

test('keeps the list toolbar while the contacts arrive', async () => {
	server.use(graphql.query('ContactPage', () => new Promise(() => {})))
	renderAt('/contacts')

	expect(await screen.findByRole('searchbox', { name: 'Search contacts…' })).toBeInTheDocument()
	expect(screen.queryByRole('row')).not.toBeInTheDocument()
})

test('serves the contacts screen at /contacts', async () => {
	renderAt('/contacts')

	expect(await screen.findByRole('heading', { level: 1, name: 'Contacts' })).toBeInTheDocument()
	expect((await rowOf('Ana Lopez')).getByText('ana@example.com')).toBeInTheDocument()
})

test('navigates to the contacts screen from the main menu', async () => {
	renderAt('/')

	await userEvent.click(await screen.findByRole('link', { name: 'Contacts' }))

	expect(await screen.findByRole('heading', { level: 1, name: 'Contacts' })).toBeInTheDocument()
})

test('says under the title what the contacts list is for', async () => {
	renderAt('/contacts')

	const subtitle = await screen.findByText('Manage the people you work with and how to reach them.')
	expect(subtitle).toHaveClass('godmin-page__subtitle')
})

test('says what the contacts list is for in Spanish', async () => {
	server.use(graphql.query('AppLocale', () => HttpResponse.json({ data: { locale: 'es-ES' } })))
	await startAppLocale()
	onTestFinished(() => resetLocale())
	renderAt('/contacts')

	const subtitle = await screen.findByText('Gestiona las personas con las que trabajas y cómo contactar con ellas.')
	expect(subtitle).toHaveClass('godmin-page__subtitle')
	expect(await screen.findByRole('searchbox', { name: 'Buscar contactos…' })).toBeInTheDocument()
})

test('draws New contact as a compact button, as a WordPress page header does', async () => {
	renderAt('/contacts')

	const add = await screen.findByRole('link', { name: 'New contact' })
	expect([...add.classList]).toEqual(buttonClasses('solid', 'compact'))
	expect(add).toHaveAttribute('href', '/contacts/new')
})

test('lines the list up with the title, out to the canvas edges', async () => {
	renderAt('/contacts')

	const search = await screen.findByRole('searchbox', { name: 'Search contacts…' })
	expect(search.closest('.godmin-page__list')).not.toBeNull()
})

test('shows an empty state when no contacts exist', async () => {
	serving([])
	renderAt('/contacts')

	expect(await screen.findByText('No contacts yet.', {}, settled)).toBeInTheDocument()
	expect(screen.getByText('Add one with New contact.').closest('.godmin-empty')).not.toBeNull()
})

test('says nobody matched when a search finds no contact', async () => {
	renderAt('/contacts?search=nobody')

	expect(await screen.findByText('No contacts found.', {}, settled)).toBeInTheDocument()
	expect(screen.queryByText('No contacts yet.')).not.toBeInTheDocument()
})

test('says nobody matched when no contact is reached on the channel picked', async () => {
	serving([maria])
	const filters = encodeURIComponent(JSON.stringify([{ field: 'channels', operator: 'isAny', value: ['email'] }]))
	renderAt(`/contacts?filters=${filters}`)

	expect(await screen.findByText('No contacts found.', {}, settled)).toBeInTheDocument()
})

test('shows the first identity of each contact under its name, in the name column', async () => {
	renderAt('/contacts')

	const row = await rowOf('Ana Lopez')
	expect(row.getByText('ana@example.com').closest('td')).toBe(row.getByText('Ana Lopez').closest('td'))
	expect(row.queryByText('184467235')).not.toBeInTheDocument()
	expect(screen.queryByRole('columnheader', { name: /Identity/ })).not.toBeInTheDocument()
})

test('shows a contact with no identity by its name alone', async () => {
	renderAt('/contacts')

	const row = await rowOf('Maria Perez')
	expect(row.getAllByRole('cell')[0]).toHaveTextContent(/^MMaria Perez$/)
})

test('draws an initials avatar before every name, hidden from screen readers', async () => {
	renderAt('/contacts')

	const avatar = (await rowOf('Ada Lovelace')).getByText('A', { selector: '.godmin-avatar' })
	expect(avatar).toHaveAttribute('aria-hidden', 'true')
	expect(avatar.parentElement).toHaveClass('dataviews-column-primary__media')
	expect((await rowOf('Maria Perez')).getByText('M', { selector: '.godmin-avatar' })).toBeInTheDocument()
})

test('links the name and the avatar of every contact to the contact', async () => {
	renderAt('/contacts')

	const links = (await rowOf('Ana Lopez')).getAllByRole('link', { name: 'Ana Lopez' })
	expect(links.map((link) => link.getAttribute('href'))).toEqual([`/contacts/${ana.id}`, `/contacts/${ana.id}`])
})

test('opens a contact from its name', async () => {
	renderAt('/contacts')

	await userEvent.click((await rowOf('Ada Lovelace')).getAllByRole('link', { name: 'Ada Lovelace' })[1])

	expect(await screen.findByRole('heading', { level: 1, name: 'Ada Lovelace' }, settled)).toBeInTheDocument()
})

test('draws one outline badge per channel a contact is reached on', async () => {
	renderAt('/contacts')

	const row = await rowOf('Ada Lovelace')
	expect(row.getAllByText(/^(Email|Phone|WhatsApp)$/).map((badge) => badge.textContent)).toEqual([
		'Phone',
		'WhatsApp',
	])
	expect([...row.getByText('WhatsApp').classList]).toEqual([...badgeClasses('none'), 'alphone-contacts__channel'])
	expect(screen.getByRole('columnheader', { name: /Channels/ })).toBeInTheDocument()
})

test('keeps the channel badges of a contact on one line, so a long name never makes the row taller', async () => {
	renderAt('/contacts')

	const badges = (await rowOf('Ana Lopez')).getAllByText(/^(Email|Phone)$/)
	expect(badges.map((badge) => badge.classList.contains('alphone-contacts__channel'))).toEqual([true, true])
	expect(badges[0].parentElement).toHaveStyle({ flexWrap: 'nowrap' })
})

test('names a channel nobody names by its key', async () => {
	serving([contactOf('04', 'Maria Perez', '2026-07-06T10:00:00Z', [['telegram', '184467235']])])
	renderAt('/contacts')

	expect((await rowOf('Maria Perez')).getByText('telegram')).toBeInTheDocument()
})

test('dates every contact in the format locale', async () => {
	renderAt('/contacts')

	expect((await rowOf('Ana Lopez')).getByText('08/07/2026')).toBeInTheDocument()
})

test('sorts the contacts by when they were created, newest first, when the address names no order', async () => {
	renderAt('/contacts')

	expect(await shownNames()).toEqual(['Ana Lopez', 'Ada Lovelace', 'Maria Perez'])
	expect(asked[0]).toMatchObject({ orderBy: 'CREATED_AT', order: 'DESC' })
	expect(screen.getByRole('columnheader', { name: /Created/ })).toHaveAttribute('aria-sort', 'descending')
})

test('sorts the contacts by name from A to Z when the address says so', async () => {
	renderAt('/contacts?sort=name&order=asc')

	expect(await shownNames()).toEqual(['Ada Lovelace', 'Ana Lopez', 'Maria Perez'])
	expect(asked[0]).toMatchObject({ orderBy: 'NAME', order: 'ASC' })
})

test('searches the contacts on the server', async () => {
	renderAt('/contacts')

	await userEvent.type(await screen.findByRole('searchbox', { name: 'Search contacts…' }), 'Lovelace')

	await waitFor(async () => expect(await shownNames()).toEqual(['Ada Lovelace']))
	expect(asked[0]).toMatchObject({ q: null })
	expect(asked.at(-1)).toMatchObject({ q: 'Lovelace', offset: 0 })
})

test('narrows the contacts to the channels the address names', async () => {
	const filters = encodeURIComponent(JSON.stringify([{ field: 'channels', operator: 'isAny', value: ['whatsapp'] }]))
	renderAt(`/contacts?filters=${filters}`)

	expect(await shownNames()).toEqual(['Ada Lovelace'])
	expect(asked[0]).toMatchObject({ channels: ['whatsapp'] })
})

test('narrows the contacts to the channel picked through the filter button', async () => {
	renderAt('/contacts')

	await userEvent.click(await screen.findByRole('button', { name: 'Add filter' }))
	await userEvent.click(await screen.findByRole('menuitem', { name: 'Channels' }))
	await userEvent.click(await screen.findByRole('option', { name: 'Email' }))

	await waitFor(async () => expect(await shownNames()).toEqual(['Ana Lopez']))
	expect(asked.at(-1)).toMatchObject({ channels: ['email'] })
})

test('offers as channels every channel the app and its plugins know', async () => {
	const filters = encodeURIComponent(JSON.stringify([{ field: 'channels', operator: 'isAny', value: [] }]))
	renderAt(`/contacts?filters=${filters}`)

	await userEvent.click(await screen.findByRole('button', { name: 'Filter' }))
	await userEvent.click(await screen.findByRole('button', { name: 'Channels', pressed: false }))

	const offered = (await screen.findAllByRole('option')).map((option) => option.textContent)
	expect(offered).toEqual(['Email', 'Phone', 'WhatsApp'])
})

test('asks for every channel while a filter names none, or names a field the list never filters', async () => {
	const filters = encodeURIComponent(
		JSON.stringify([
			{ field: 'channels', operator: 'isAny' },
			{ field: 'name', operator: 'isAny', value: ['Ana Lopez'] },
		]),
	)
	renderAt(`/contacts?filters=${filters}`)

	expect(await shownNames()).toEqual(['Ana Lopez', 'Ada Lovelace', 'Maria Perez'])
	expect(asked[0]).toMatchObject({ channels: null })
})

test('pages the contacts by the page size the admin settings name', async () => {
	paging([2, 4], 2)
	renderAt('/contacts')

	expect(await shownNames()).toEqual(['Ana Lopez', 'Ada Lovelace'])
	expect(asked[0]).toMatchObject({ limit: 2, offset: 0 })
	await userEvent.click(screen.getByRole('button', { name: 'Next page' }))

	await waitFor(async () => expect(await shownNames()).toEqual(['Maria Perez']))
	expect(asked.at(-1)).toMatchObject({ limit: 2, offset: 2 })
})

test('opens on the page the address names', async () => {
	paging([2, 4], 2)
	renderAt('/contacts?page=2')

	await waitFor(async () => expect(await shownNames()).toEqual(['Maria Perez']))
	expect(asked).toHaveLength(1)
})

test('offers as page sizes the ones the admin settings name', async () => {
	paging([2, 4], 2)
	renderAt('/contacts')

	await screen.findByRole('table')
	await userEvent.click(screen.getByRole('button', { name: 'View options' }))

	const sizes = within(await screen.findByRole('radiogroup', { name: 'Items per page' }))
	expect(sizes.getAllByRole('radio').map((size) => size.textContent)).toEqual(['2', '4'])
})

test('holds a page the address asks for under the contact page cap', async () => {
	paging([2, 4], 2, 3)
	renderAt('/contacts?perPage=10')

	await screen.findByRole('table')
	expect(asked[0]).toMatchObject({ limit: 3, offset: 0 })
})

test('waits for the page size before it asks for contacts', async () => {
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
	renderAt('/contacts?page=2')

	await waitFor(async () => expect(await shownNames()).toEqual(['Maria Perez']), { timeout: 3000 })
	expect(asked).toEqual([expect.objectContaining({ limit: 2, offset: 2 })])
})

/**
 * Makes the admin settings unreadable.
 */
function unsized() {
	server.use(
		graphql.query('AdminSettings', () => HttpResponse.json({ data: null, errors: [{ message: 'internal error' }] })),
	)
}

test('pages by the size the graph serves when the page size could not be read', async () => {
	unsized()
	serving([ana, ada, maria], 2)
	renderAt('/contacts')

	await waitFor(async () => expect(await shownNames()).toEqual(['Ana Lopez', 'Ada Lovelace']))
	expect(asked[0]).toMatchObject({ limit: null, offset: 0 })
	await userEvent.click(screen.getByRole('button', { name: 'Next page' }))

	await waitFor(async () => expect(await shownNames()).toEqual(['Maria Perez']))
	expect(asked.at(-1)).toMatchObject({ limit: null, offset: 2 })
})

test('opens on the address page by the size the graph serves when the page size could not be read', async () => {
	unsized()
	serving([ana, ada, maria], 2)
	renderAt('/contacts?page=2')

	await waitFor(async () => expect(await shownNames()).toEqual(['Maria Perez']), settled)
	expect(asked.at(-1)).toMatchObject({ limit: null, offset: 2 })
})

test('asks the size the graph serves when the page size could not be read, even with one in the address', async () => {
	unsized()
	serving([ana, ada, maria], 2)
	renderAt('/contacts?perPage=10')

	await waitFor(async () => expect(await shownNames()).toEqual(['Ana Lopez', 'Ada Lovelace']), settled)
	expect(asked[0]).toMatchObject({ limit: null, offset: 0 })
})

test('shows every contact on one page when the graph serves them all and the page size could not be read', async () => {
	unsized()
	renderAt('/contacts')

	await waitFor(async () => expect(await shownNames()).toEqual(['Ana Lopez', 'Ada Lovelace', 'Maria Perez']))
	expect(screen.queryByRole('button', { name: 'Next page' })).not.toBeInTheDocument()
})

test('counts no contacts and ticks none, because no bulk action exists yet', async () => {
	renderAt('/contacts')

	await rowOf('Ana Lopez')
	expect(screen.queryByText('3 Items')).not.toBeInTheDocument()
	expect(screen.queryByRole('checkbox')).not.toBeInTheDocument()
})

test('reads the contacts afresh each time the list opens', async () => {
	renderAt('/contacts')
	await rowOf('Ana Lopez')
	await userEvent.click(screen.getByRole('link', { name: 'New contact' }))
	await screen.findByRole('heading', { level: 1, name: 'New contact' })
	serving([contactOf('05', 'Grace Hopper', '2026-07-09T10:00:00Z'), ana, ada, maria])

	await userEvent.click(screen.getByRole('link', { name: 'Contacts' }))

	await waitFor(async () => expect((await shownNames())[0]).toBe('Grace Hopper'))
})

test('reports when contacts cannot be loaded', async () => {
	server.use(
		graphql.query('ContactPage', () => HttpResponse.json({ data: null, errors: [{ message: 'internal error' }] })),
	)
	renderAt('/contacts')

	expect(await screen.findByRole('alert', {}, settled)).toHaveTextContent('Contacts could not be loaded.')
	expect(screen.queryByText('internal error')).not.toBeInTheDocument()
})

test('drops the session when the contacts request is unauthorized', async () => {
	server.use(
		graphql.query('ContactPage', () =>
			HttpResponse.json({
				data: null,
				errors: [{ message: 'no session', extensions: { code: 'UNAUTHENTICATED' } }],
			}),
		),
	)

	const client = renderAt('/contacts')

	await waitFor(() => expect(client.getQueryData(sessionQueryKey)).toBeNull())
})

test('lays the contacts out as a list on a phone', async () => {
	setViewport({ matches: true })
	renderAt('/contacts')

	expect(await screen.findByText('Ada Lovelace', {}, settled)).toBeInTheDocument()
	expect(screen.queryByRole('table')).not.toBeInTheDocument()
})

test('draws the initials avatar before every name in the list a phone shows', async () => {
	setViewport({ matches: true })
	renderAt('/contacts')

	const item = (await screen.findByRole('button', { name: 'Ada Lovelace' }, settled)).closest(
		'[role="row"]',
	) as HTMLElement
	const avatar = within(item).getByText('A', { selector: '.godmin-avatar' })
	expect(avatar.parentElement).toHaveClass('dataviews-view-list__media-wrapper')
})

test('opens the contact a tap lands on in the list a phone shows', async () => {
	setViewport({ matches: true })
	renderAt('/contacts')

	await userEvent.click(await screen.findByRole('button', { name: 'Ada Lovelace' }, settled))

	expect(await screen.findByRole('heading', { level: 1, name: 'Ada Lovelace' }, settled)).toBeInTheDocument()
})

test('offers Rename before Add task in the row menu, the modal first', async () => {
	renderAt('/contacts')

	await userEvent.click((await rowOf('Ana Lopez')).getByRole('button', { name: 'Actions' }))

	const offered = (await screen.findAllByRole('menuitem')).map((item) => item.textContent)
	expect(offered).toEqual(['Rename', 'Add task'])
})

/**
 * Answers the rename mutation, renaming the contact it names in the directory.
 * @returns The names the mutation received.
 */
function writesName() {
	const names: string[] = []
	server.use(
		graphql.mutation('RenameContact', ({ variables }) => {
			const name = String(variables.name)
			names.push(name)
			held = held.map((row) => (row.id === variables.id ? { ...row, name } : row))
			return HttpResponse.json({
				data: { renameContact: { __typename: 'Contact', id: String(variables.id), name } },
			})
		}),
	)
	return names
}

test('renames a contact in a small modal and confirms it with a toast', async () => {
	const names = writesName()
	renderAt('/contacts?sort=name&order=asc')

	await act('Ana Lopez', 'Rename')
	const dialog = within(await screen.findByRole('dialog', { name: 'Rename contact' }))
	const name = dialog.getByRole('textbox', { name: 'Name' })
	expect(name).toHaveValue('Ana Lopez')
	await userEvent.clear(name)
	await userEvent.type(name, 'Zoe Lopez')
	await userEvent.click(dialog.getByRole('button', { name: 'Rename' }))

	expect(await screen.findByText('Contact renamed.')).toBeInTheDocument()
	expect(names).toEqual(['Zoe Lopez'])
	await waitFor(() => expect(screen.queryByRole('dialog')).not.toBeInTheDocument())
	await waitFor(async () => expect(await shownNames()).toEqual(['Ada Lovelace', 'Maria Perez', 'Zoe Lopez']))
})

test('opens the rename in a small modal with Cancel as a text button', async () => {
	renderAt('/contacts')

	await act('Ana Lopez', 'Rename')

	const dialog = await screen.findByRole('dialog', { name: 'Rename contact' })
	expect(dialog).toHaveClass('has-size-small')
	expect([...within(dialog).getByRole('button', { name: 'Cancel' }).classList]).toEqual(buttonClasses('minimal'))
})

test('puts the cursor in the name when the rename opens, as the WordPress rename does', async () => {
	const laidOut = vi
		.spyOn(HTMLElement.prototype, 'getClientRects')
		.mockReturnValue([new DOMRect(0, 0, 10, 10)] as unknown as DOMRectList)
	onTestFinished(() => laidOut.mockRestore())
	renderAt('/contacts')

	await act('Ana Lopez', 'Rename')

	const dialog = within(await screen.findByRole('dialog', { name: 'Rename contact' }))
	await waitFor(() => expect(dialog.getByRole('textbox', { name: 'Name' })).toHaveFocus())
})

test('waits for another name before it renames', async () => {
	renderAt('/contacts')

	await act('Ana Lopez', 'Rename')
	const dialog = within(await screen.findByRole('dialog', { name: 'Rename contact' }))
	const rename = dialog.getByRole('button', { name: 'Rename' })
	expect(rename).toHaveAttribute('aria-disabled', 'true')

	await userEvent.clear(dialog.getByRole('textbox', { name: 'Name' }))
	await userEvent.type(dialog.getByRole('textbox', { name: 'Name' }), '   ')

	expect(rename).toHaveAttribute('aria-disabled', 'true')
})

test('closes the rename modal without writing when the reader cancels', async () => {
	const names = writesName()
	renderAt('/contacts')

	await act('Ana Lopez', 'Rename')
	await userEvent.click(await screen.findByRole('button', { name: 'Cancel' }))

	await waitFor(() => expect(screen.queryByRole('dialog')).not.toBeInTheDocument())
	expect(names).toEqual([])
})

/**
 * Renames Ana Lopez through the row modal, answering with the given failure.
 * @param error - The error the rename mutation answers with.
 * @returns The modal queries.
 */
async function failRename(error: Record<string, unknown>) {
	server.use(graphql.mutation('RenameContact', () => HttpResponse.json({ data: null, errors: [error] })))
	renderAt('/contacts')
	await act('Ana Lopez', 'Rename')
	const dialog = within(await screen.findByRole('dialog', { name: 'Rename contact' }))
	await userEvent.type(dialog.getByRole('textbox', { name: 'Name' }), ' Ltd')
	await userEvent.click(dialog.getByRole('button', { name: 'Rename' }))
	return dialog
}

test('shows why a contact could not be renamed inside the modal', async () => {
	configureAppErrorText()

	const dialog = await failRename({
		message: 'contact: empty name',
		extensions: { code: 'VALIDATION', reason: 'contact_name_required' },
	})

	expect(await dialog.findByRole('alert')).toHaveTextContent('A contact needs a name.')
	expect(screen.queryByText('Contact renamed.')).not.toBeInTheDocument()
})

test('says plainly that a contact could not be renamed when the server names no reason', async () => {
	const dialog = await failRename({ message: 'internal error' })

	expect(await dialog.findByRole('alert')).toHaveTextContent('The contact could not be renamed.')
	expect(screen.queryByText('internal error')).not.toBeInTheDocument()
})

test('opens a new task for the contact from its row', async () => {
	server.use(
		graphql.query('ContactName', ({ variables }) =>
			HttpResponse.json({
				data: { contact: { __typename: 'Contact', id: String(variables.id), name: 'Ana Lopez' } },
			}),
		),
	)
	renderAt('/contacts')

	await act('Ana Lopez', 'Add task')

	expect(await screen.findByRole('heading', { level: 1, name: 'New task' })).toBeInTheDocument()
	expect(await screen.findByRole('button', { name: 'Remove Ana Lopez' })).toBeInTheDocument()
})
