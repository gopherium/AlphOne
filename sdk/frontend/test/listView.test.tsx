// SPDX-License-Identifier: AGPL-3.0-or-later

import { setViewport } from '@gopherium/godmin/testing'
import {
	RouterProvider,
	createMemoryHistory,
	createRootRoute,
	createRoute,
	createRouter,
} from '@tanstack/react-router'
import { act, fireEvent, render, screen } from '@testing-library/react'
import { expect, test } from 'vitest'

import { listSearch, openOnTap, useListView } from '../index'
import type { ListDefaults, ListView } from '../index'

const defaults: ListDefaults = {
	fields: ['email', 'status'],
	titleField: 'name',
	sort: { field: 'name', direction: 'asc' },
	perPage: 20,
}

test('listSearch keeps every part of a view the address may carry', () => {
	const filters = [{ field: 'status', operator: 'isAny', value: ['invited'] }]

	const kept = listSearch({
		search: 'ada',
		page: 2,
		perPage: 50,
		sort: 'email',
		order: 'desc',
		filters,
		tab: 'other',
	})

	expect(kept).toEqual({ search: 'ada', page: 2, perPage: 50, sort: 'email', order: 'desc', filters })
})

test('listSearch drops what no list view could hold', () => {
	const kept = listSearch({
		search: '',
		page: 0,
		perPage: 2.5,
		sort: 7,
		order: 'sideways',
		filters: [{ field: 'status' }, 'role', null, { field: 'role', operator: 'isAny', value: ['admin'] }],
	})

	expect(kept).toEqual({ filters: [{ field: 'role', operator: 'isAny', value: ['admin'] }] })
})

test('listSearch drops filters that are not a list', () => {
	expect(listSearch({ filters: { field: 'status' } })).toEqual({})
})

/** Changes a shown view into the one a test writes. */
type change = (view: ListView['view']) => ListView['view']

/**
 * Renders the view the address holds, and a button writing the changed one.
 * @param props - The change the button writes.
 * @returns The probe.
 */
function Probe({ next, opening }: { next: change; opening: ListDefaults }) {
	const list = useListView(opening)
	return (
		<>
			<output aria-label="view">{JSON.stringify(list.view)}</output>
			<output aria-label="layouts">{JSON.stringify(list.defaultLayouts)}</output>
			<output aria-label="selection">{JSON.stringify(list.selection)}</output>
			<button type="button" onClick={() => list.onChangeView(next(list.view))}>
				Change
			</button>
			<button type="button" onClick={() => list.onChangeSelection(['1'])}>
				Tick
			</button>
		</>
	)
}

/**
 * Renders the probe screen at the given address.
 * @param path - The address the memory history starts on.
 * @param next - The change the probe writes when asked.
 * @param opening - What the list opens on.
 * @returns The router the probe renders under.
 */
function renderProbe(path: string, next: change = (view) => view, opening: ListDefaults = defaults) {
	const rootRoute = createRootRoute()
	const peopleRoute = createRoute({
		getParentRoute: () => rootRoute,
		path: '/people',
		validateSearch: listSearch,
		component: () => <Probe next={next} opening={opening} />,
	})
	const router = createRouter({
		routeTree: rootRoute.addChildren([peopleRoute]),
		history: createMemoryHistory({ initialEntries: [path] }),
	})
	render(<RouterProvider router={router} />)
	return router
}

/**
 * Returns what one probe output shows.
 * @param name - The output to read.
 * @returns The parsed value.
 */
async function shown(name: 'view' | 'layouts' | 'selection') {
	return JSON.parse((await screen.findByRole('status', { name })).textContent ?? '') as unknown
}

test('a list opens on its defaults when the address says nothing', async () => {
	renderProbe('/people')

	expect(await shown('view')).toEqual({
		type: 'table',
		fields: ['email', 'status'],
		titleField: 'name',
		search: '',
		filters: [],
		sort: { field: 'name', direction: 'asc' },
		page: 1,
		perPage: 20,
	})
	expect(await shown('layouts')).toEqual({ table: {} })
})

test('a list sets the field it names as the description under the title', async () => {
	renderProbe('/people', undefined, { ...defaults, fields: ['status'], descriptionField: 'email' })

	expect(await shown('view')).toMatchObject({ titleField: 'name', descriptionField: 'email', fields: ['status'] })
})

test('a list sets the field it names as the media before the title', async () => {
	renderProbe('/people', undefined, { ...defaults, descriptionField: 'email', mediaField: 'avatar' })

	expect(await shown('view')).toMatchObject({ titleField: 'name', descriptionField: 'email', mediaField: 'avatar' })
})

test('a list keeps the media a reader hid while it stays open, out of the address', async () => {
	const router = renderProbe('/people', (view) => ({ ...view, showMedia: false }), { ...defaults, mediaField: 'avatar' })

	fireEvent.click(await screen.findByRole('button', { name: 'Change' }))

	await screen.findByText(/showMedia/)
	expect(await shown('view')).toMatchObject({ mediaField: 'avatar', showMedia: false })
	expect(router.state.location.search).toEqual({})
})

test('a list reads its search, filters, sort and page from the address', async () => {
	const filters = encodeURIComponent(JSON.stringify([{ field: 'status', operator: 'isAny', value: ['invited'] }]))
	renderProbe(`/people?search=ada&page=2&perPage=50&sort=email&order=desc&filters=${filters}`)

	expect(await shown('view')).toMatchObject({
		search: 'ada',
		filters: [{ field: 'status', operator: 'isAny', value: ['invited'] }],
		sort: { field: 'email', direction: 'desc' },
		page: 2,
		perPage: 50,
	})
})

test('a changed view lands in the address without the parts left at their defaults', async () => {
	const router = renderProbe('/people?search=ada', (view) => ({
		...view,
		search: 'maria',
		page: 3,
		sort: { field: 'name', direction: 'desc' },
	}))

	fireEvent.click(await screen.findByRole('button', { name: 'Change' }))

	await screen.findByText(/maria/)
	expect(await shown('view')).toMatchObject({ search: 'maria', page: 3, sort: { field: 'name', direction: 'desc' } })
	expect(router.state.location.search).toEqual({ search: 'maria', page: 3, order: 'desc' })
})

test('a view back at its defaults leaves a bare address', async () => {
	const router = renderProbe('/people?search=ada&page=2&sort=email&perPage=50', (view) => ({
		...view,
		search: '',
		page: 1,
		perPage: 20,
		sort: undefined,
		filters: [],
	}))

	fireEvent.click(await screen.findByRole('button', { name: 'Change' }))

	await screen.findByText(/"page":1/)
	expect(await shown('view')).toMatchObject({ search: '', page: 1, sort: { field: 'name', direction: 'asc' } })
	expect(router.state.location.search).toEqual({})
})

test('a changed view replaces the address, so Back leaves the list', async () => {
	const router = renderProbe('/people', (view) => ({ ...view, search: 'maria' }))

	fireEvent.click(await screen.findByRole('button', { name: 'Change' }))

	await screen.findByText(/maria/)
	expect(router.history.length).toBe(1)
})

test('a list keeps the columns and density a reader picked while it stays open, out of the address', async () => {
	const router = renderProbe('/people', (view) => ({
		...view,
		fields: ['email'],
		layout: { density: 'compact' },
	}))

	fireEvent.click(await screen.findByRole('button', { name: 'Change' }))

	await screen.findByText(/compact/)
	expect(await shown('view')).toMatchObject({ fields: ['email'], layout: { density: 'compact' } })
	expect(router.state.location.search).toEqual({})
})

test('a phone lays a list out as a list, not a table', async () => {
	setViewport({ matches: true })
	renderProbe('/people')

	expect(await shown('view')).toMatchObject({ type: 'list' })
	expect(await shown('layouts')).toEqual({ list: {} })
})

test('a phone lays a list out with the fields the list names for a phone', async () => {
	setViewport({ matches: true })
	renderProbe('/people', undefined, { ...defaults, phoneFields: ['status'] })

	expect(await shown('view')).toMatchObject({ type: 'list', fields: ['status'] })
})

test('a table keeps its columns when the list names other fields for a phone', async () => {
	renderProbe('/people', undefined, { ...defaults, phoneFields: ['status'] })

	expect(await shown('view')).toMatchObject({ type: 'table', fields: ['email', 'status'] })
})

test('a phone keeps the columns when the list names no fields for a phone', async () => {
	setViewport({ matches: true })
	renderProbe('/people')

	expect(await shown('view')).toMatchObject({ type: 'list', fields: ['email', 'status'] })
})

test('a table holds the rows a reader ticks', async () => {
	renderProbe('/people')

	fireEvent.click(await screen.findByRole('button', { name: 'Tick' }))

	expect(await shown('selection')).toEqual(['1'])
})

test('a list on a phone selects nothing, so a tap on a row changes nothing', async () => {
	setViewport({ matches: true })
	renderProbe('/people')

	fireEvent.click(await screen.findByRole('button', { name: 'Tick' }))

	expect(await shown('selection')).toEqual([])
})

test('a list drops the rows ticked in the table once the layout flips', async () => {
	renderProbe('/people')
	fireEvent.click(await screen.findByRole('button', { name: 'Tick' }))
	await screen.findByText('["1"]')

	act(() => setViewport({ matches: true }))
	act(() => setViewport({ matches: false }))

	expect(await shown('view')).toMatchObject({ type: 'table' })
	expect(await shown('selection')).toEqual([])
})

/**
 * Builds the list view a screen holds in the given layout, its ticks held by the given handler.
 * @param type - The layout the viewport takes.
 * @param onChangeSelection - The handler holding the ticks.
 * @returns The list view.
 */
function listIn(type: 'table' | 'list', onChangeSelection: (selection: string[]) => void): ListView {
	return {
		view: { type, fields: [] },
		onChangeView: () => {},
		defaultLayouts: { [type]: {} },
		selection: [],
		onChangeSelection,
	}
}

test('a tap in the list a phone shows opens the record it lands on', () => {
	const ticked: string[][] = []
	const opened: string[] = []

	openOnTap(listIn('list', (selection) => ticked.push(selection)), (id) => opened.push(id))(['7'])

	expect({ ticked, opened }).toEqual({ ticked: [], opened: ['7'] })
})

test('a tick in a table stays a selection and opens nothing', () => {
	const ticked: string[][] = []
	const opened: string[] = []

	openOnTap(listIn('table', (selection) => ticked.push(selection)), (id) => opened.push(id))(['7'])

	expect({ ticked, opened }).toEqual({ ticked: [['7']], opened: [] })
})
