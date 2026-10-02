// SPDX-License-Identifier: AGPL-3.0-or-later

import { DENSE_BREAKPOINT, useMediaQuery } from '@gopherium/godmin'
import { useNavigate, useSearch } from '@tanstack/react-router'
import type { Filter, SortDirection, SupportedLayouts, View } from '@wordpress/dataviews'
import { useState } from 'react'

/** ListSearch is the part of a list view the address carries. */
export interface ListSearch {
	/** search is the words the list is searched for. */
	search?: string
	/** page is the page shown, counted from one. */
	page?: number
	/** perPage is how many rows a page holds. */
	perPage?: number
	/** sort names the field the rows are sorted by. */
	sort?: string
	/** order is the direction the rows are sorted in. */
	order?: SortDirection
	/** filters are the filters narrowing the rows. */
	filters?: Filter[]
}

/** ListDefaults names what a list opens on when the address says nothing. */
export interface ListDefaults {
	/** fields lists the columns shown beside the title. */
	fields: string[]
	/** phoneFields lists the fields the list a phone shows carries beside the title, the columns when absent. */
	phoneFields?: string[]
	/** titleField names the field that links to the record. */
	titleField: string
	/** descriptionField names the field shown under the title, none when absent. */
	descriptionField?: string
	/** mediaField names the field drawn before the title, such as an avatar, none when absent. */
	mediaField?: string
	/** sort is the order a list opens in. */
	sort: { field: string; direction: SortDirection }
	/** perPage is how many rows a page holds when the address names none, every row while it is unknown. */
	perPage?: number
}

/** ListView carries the view the address holds beside the controls DataViews takes. */
export interface ListView {
	/** view is the view the address holds, laid out for the viewport. */
	view: View
	/** onChangeView writes a changed view into the address, holding the parts it leaves out in memory. */
	onChangeView: (view: View) => void
	/** defaultLayouts offers the one layout the viewport takes. */
	defaultLayouts: SupportedLayouts
	/** selection names the rows ticked in the table, always none in the list a phone shows. */
	selection: string[]
	/** onChangeSelection holds the rows ticked in the table, and drops the tap a phone list reports. */
	onChangeSelection: (selection: string[]) => void
}

/** ListShape is the part of a list view a reader changes that the address leaves out, such as the columns. */
type ListShape = Record<string, unknown>

/** ListLayout is the layout a viewport lays a list out in. */
type ListLayout = 'table' | 'list'

/** ListSelection is the selection a list holds beside the control that changes it. */
type ListSelection = Pick<ListView, 'selection' | 'onChangeSelection'>

/** noSelection is the selection a list opens on and the one a phone list always holds. */
const noSelection: string[] = []

/** inert is the selection of a phone list, always none and never changed. */
const inert: ListSelection = { selection: noSelection, onChangeSelection: () => {} }

/** phoneQuery matches the viewports that lay a list out as a list rather than a table. */
const phoneQuery = `(max-width: ${DENSE_BREAKPOINT - 1}px)`

/** addressed names the parts of a view the address or the viewport decides. */
const addressed = new Set(['type', 'search', 'filters', 'sort', 'page', 'perPage'])

/** listKeys names the address keys a list view owns. */
const listKeys = new Set(['search', 'page', 'perPage', 'sort', 'order', 'filters'])

/** noShapes is the shape each layout holds before a reader changes one. */
const noShapes: Record<ListLayout, ListShape> = { table: {}, list: {} }

/**
 * Returns the part of a view the address leaves out.
 * @param view - The view the list shows.
 * @returns The columns, layout and every other part the reader changed.
 */
function shapeOf(view: View): ListShape {
	return Object.fromEntries(Object.entries(view).filter(([key]) => !addressed.has(key)))
}

/**
 * Returns the value when it is a whole number above zero.
 * @param value - The raw address value.
 * @returns The number, or undefined.
 */
function positive(value: unknown): number | undefined {
	return Number.isInteger(value) && (value as number) > 0 ? (value as number) : undefined
}

/**
 * Returns the value when it is a string holding something.
 * @param value - The raw address value.
 * @returns The string, or undefined.
 */
function filled(value: unknown): string | undefined {
	return typeof value === 'string' && value !== '' ? value : undefined
}

/**
 * Returns the value when it names a sort direction.
 * @param value - The raw address value.
 * @returns The direction, or undefined.
 */
function direction(value: unknown): SortDirection | undefined {
	return value === 'asc' || value === 'desc' ? value : undefined
}

/**
 * Reports whether one raw entry is a filter a list can apply.
 * @param entry - One raw entry of the address filters.
 * @returns Whether it names a field and an operator.
 */
function isFilter(entry: unknown): entry is Filter {
	const held = entry as Partial<Filter> | null
	return typeof held?.field === 'string' && typeof held.operator === 'string'
}

/**
 * Returns the filters an address value holds, keeping only the ones a list can apply.
 * @param value - The raw address value.
 * @returns The filters, or undefined when none remain.
 */
function filtersOf(value: unknown): Filter[] | undefined {
	if (!Array.isArray(value)) {
		return undefined
	}
	const kept = value.filter(isFilter).map(({ field, operator, value: chosen }) => ({ field, operator, value: chosen }))
	return kept.length > 0 ? kept : undefined
}

/**
 * Returns the object without its undefined entries.
 * @param search - The search with possibly undefined entries.
 * @returns The search holding only what was set.
 */
function withoutGaps(search: ListSearch): ListSearch {
	return Object.fromEntries(Object.entries(search).filter(([, value]) => value !== undefined)) as ListSearch
}

/**
 * Keeps the parts of a list view an address may carry, dropping anything malformed.
 * @param raw - The search the router parsed from the address.
 * @returns The list search.
 */
export function listSearch(raw: Record<string, unknown>): ListSearch {
	return withoutGaps({
		search: filled(raw.search),
		page: positive(raw.page),
		perPage: positive(raw.perPage),
		sort: filled(raw.sort),
		order: direction(raw.order),
		filters: filtersOf(raw.filters),
	})
}

/**
 * Returns the fields a list opens on in the layout the viewport takes.
 * @param defaults - What the list opens on.
 * @param type - The layout the viewport takes.
 * @returns The fields shown beside the title.
 */
function fieldsFor(defaults: ListDefaults, type: ListLayout): string[] {
	return type === 'list' ? (defaults.phoneFields ?? defaults.fields) : defaults.fields
}

/**
 * Builds the view a list shows from its address, the reader's other changes and its defaults.
 * @param search - The list search the address carries.
 * @param defaults - What the list opens on.
 * @param shape - The part of the view the reader changed that the address leaves out.
 * @param type - The layout the viewport takes.
 * @returns The view.
 */
function viewOf(search: ListSearch, defaults: ListDefaults, shape: ListShape, type: ListLayout): View {
	return {
		fields: fieldsFor(defaults, type),
		titleField: defaults.titleField,
		descriptionField: defaults.descriptionField,
		mediaField: defaults.mediaField,
		...shape,
		type,
		search: search.search ?? '',
		filters: search.filters ?? [],
		sort: { field: search.sort ?? defaults.sort.field, direction: search.order ?? defaults.sort.direction },
		page: search.page ?? 1,
		perPage: search.perPage ?? defaults.perPage,
	}
}

/**
 * Returns the value unless it matches what the list opens on.
 * @param value - The value the view holds.
 * @param opening - The value the list opens on.
 * @returns The value, or undefined when it is the opening one.
 */
function unlessOpening<T>(value: T, opening: T): T | undefined {
	return value === opening ? undefined : value
}

/**
 * Builds the address search a view is written to, leaving out what matches the defaults.
 * @param view - The view the list shows.
 * @param defaults - What the list opens on.
 * @returns The list search.
 */
function searchOf(view: View, defaults: ListDefaults): ListSearch {
	const sort = view.sort ?? defaults.sort
	return withoutGaps({
		search: filled(view.search),
		page: unlessOpening(view.page, 1),
		perPage: unlessOpening(view.perPage, defaults.perPage),
		sort: unlessOpening(sort.field, defaults.sort.field),
		order: unlessOpening(sort.direction, defaults.sort.direction),
		filters: filtersOf(view.filters),
	})
}

/**
 * Returns the address search without the keys a list view owns.
 * @param held - The search the address holds.
 * @returns The choices of the screen around the list.
 */
function besideList(held: Record<string, unknown>): Record<string, unknown> {
	return Object.fromEntries(Object.entries(held).filter(([key]) => !listKeys.has(key)))
}

/**
 * Holds the rows ticked in a table, none in a phone list, and drops them when the layout flips.
 * @param layout - The layout the viewport takes.
 * @returns The selection beside the control that changes it.
 */
function useSelection(layout: ListLayout): ListSelection {
	const [ticked, setTicked] = useState(noSelection)
	const [tickedIn, setTickedIn] = useState(layout)
	if (tickedIn !== layout) {
		setTickedIn(layout)
		setTicked(noSelection)
	}
	return layout === 'list' ? inert : { selection: ticked, onChangeSelection: setTicked }
}

/**
 * Holds a list view in the address and its columns in memory.
 * @param defaults - What the list opens on.
 * @returns The view beside the controls DataViews takes.
 */
export function useListView(defaults: ListDefaults): ListView {
	const raw = useSearch({ strict: false }) as Record<string, unknown>
	const navigate = useNavigate()
	const phone = useMediaQuery(phoneQuery)
	const layout: ListLayout = phone ? 'list' : 'table'
	const [shapes, setShapes] = useState(noShapes)
	const selection = useSelection(layout)
	return {
		view: viewOf(listSearch(raw), defaults, shapes[layout], layout),
		onChangeView: (view) => {
			setShapes((held) => ({ ...held, [layout]: shapeOf(view) }))
			void navigate({
				to: '.',
				search: (held: Record<string, unknown>) => ({ ...besideList(held), ...searchOf(view, defaults) }),
				replace: true,
			})
		},
		defaultLayouts: phone ? { list: {} } : { table: {} },
		...selection,
	}
}

/**
 * Returns the selection handler a list hands DataViews, opening the record a tap lands on in the list a phone shows.
 * @param list - The list view the screen holds.
 * @param open - Opens the record with the given id.
 * @returns The handler.
 */
export function openOnTap(list: ListView, open: (id: string) => void): ListView['onChangeSelection'] {
	if (list.view.type !== 'list') {
		return list.onChangeSelection
	}
	return ([id]) => open(id)
}
