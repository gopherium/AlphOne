// SPDX-License-Identifier: AGPL-3.0-or-later

import { __, useGraph, useGraphMutation, useToaster } from '@alphone/frontend-sdk'
import type { GraphFailure } from '@alphone/frontend-sdk'
import { useState } from 'react'

import type { EntryText } from './cellText'
import { neighbour, typedEntry } from './entries'
import type { RepeaterRow, StoredEntry } from './entries'
import { entryMessage, outcomeOf, refetchAfter } from './entryOutcome'
import { whenFocusStayed } from './focus'
import { deleteContactFieldEntryMutation, updateContactFieldEntryMutation } from './operations'

/** RowFocus names the button of one row focus moves to. */
export interface RowFocus {
	id: string
	on: 'first' | 'remove' | 'keep'
}

/** RowMode is how one row shows. */
type RowMode = 'show' | 'edit' | 'confirm'

/** RowState is how one row shows and what it answers to. */
export interface RowState {
	mode: RowMode
	pending: boolean
	locked: boolean
	focus: RowFocus | undefined
}

/** EntryHandlers are the actions the buttons of a row run. */
export interface EntryHandlers {
	edit: (id: string) => void
	cancel: (id: string) => void
	save: (id: string, draft: EntryText) => Promise<void>
	remove: (id: string) => void
	keep: (id: string) => void
	confirm: (id: string) => Promise<void>
}

/** GoneHandler reports a repeater the catalogue no longer holds, and where focus was when the call began. */
export type GoneHandler = (message: string, from: Element | null) => void

/** SettleSteps are what a settled call does once it succeeds and once it is plainly refused. */
interface SettleSteps {
	fallback: string
	done: () => void
	refused: () => void
}

/**
 * Holds how many entries a repeater draws, which a row open or taking the focus raises and Load more raises by a page.
 * @param entries - Every entry.
 * @param firstPage - How many entries the first page draws.
 * @param kept - The rows that stay drawn: the one open and the one taking the focus.
 * @returns The entries drawn and the way to open one more page.
 */
function useDrawnEntries(entries: readonly StoredEntry[], firstPage: number, kept: readonly (string | undefined)[]) {
	const [held, setHeld] = useState(0)
	const reach = Math.max(...kept.map((id) => entries.findIndex((entry) => entry.id === id) + 1))
	if (reach > held) {
		setHeld(reach)
	}
	return {
		listed: entries.slice(0, Math.max(firstPage, held, reach)),
		loadMore: () => setHeld((rows) => Math.max(rows, firstPage) + firstPage),
	}
}

/**
 * Holds the state the rows of one repeater share, and the actions that change it.
 * @param props - The contact, the repeater, every entry, the size of a first page and the report of a repeater gone.
 * @returns The failure, the add focus count, the add hook, the actions, each row's state, the rows and Load more.
 */
export function useEntryActions({
	contactId,
	field,
	entries,
	firstPage,
	onGone,
}: {
	contactId: string
	field: RepeaterRow
	entries: readonly StoredEntry[]
	firstPage: number
	onGone: GoneHandler
}) {
	const [open, setOpen] = useState<{ id: string; as: 'edit' | 'confirm' } | null>(null)
	const [pendingID, setPendingID] = useState('')
	const [settling, setSettling] = useState<readonly string[]>([])
	const [failure, setFailure] = useState('')
	const [rowFocus, setRowFocus] = useState<RowFocus | null>(null)
	const [addFocus, setAddFocus] = useState(0)
	const { listed, loadMore } = useDrawnEntries(entries, firstPage, [open?.id, rowFocus?.id])
	const [, runUpdate] = useGraphMutation(updateContactFieldEntryMutation)
	const [, runDelete] = useGraphMutation(deleteContactFieldEntryMutation)
	const graph = useGraph()
	const toaster = useToaster()

	const focusAdd = () => setAddFocus((count) => count + 1)

	const added = (from: Element | null) => {
		setFailure('')
		whenFocusStayed(from, focusAdd)
	}

	const leave = (id: string, from: Element | null) => {
		const next = neighbour(listed, id, settling)
		setOpen(null)
		setSettling((held) => [...held, id])
		whenFocusStayed(from, () => (next === '' ? focusAdd() : setRowFocus({ id: next, on: 'first' })))
	}

	const settle = (id: string, from: Element | null, error: GraphFailure | undefined, steps: SettleSteps) => {
		refetchAfter(graph, error)
		const outcome = outcomeOf(error)
		if (outcome === 'done') {
			steps.done()
			return
		}
		const message = entryMessage(error as GraphFailure, steps.fallback)
		if (outcome === 'field-gone') {
			onGone(message, from)
			return
		}
		setFailure(message)
		if (outcome === 'entry-gone') {
			leave(id, from)
			return
		}
		steps.refused()
	}

	const on: EntryHandlers = {
		edit: (id) => {
			setOpen({ id, as: 'edit' })
			setFailure('')
		},
		cancel: (id) => {
			setOpen(null)
			setFailure('')
			setRowFocus({ id, on: 'first' })
		},
		save: async (id, draft) => {
			const from = document.activeElement
			setFailure('')
			setPendingID(id)
			const result = await runUpdate({
				contactId,
				field: field.name,
				entryId: id,
				entry: typedEntry(field.subFields, draft),
			})
			setPendingID('')
			settle(id, from, result.error, {
				fallback: __('The entry could not be saved.', 'alphone-fields'),
				done: () => {
					setOpen(null)
					toaster.show(__('Entry saved.', 'alphone-fields'))
					whenFocusStayed(from, () => setRowFocus({ id, on: 'first' }))
				},
				refused: () => {},
			})
		},
		remove: (id) => {
			setOpen({ id, as: 'confirm' })
			setFailure('')
			setRowFocus({ id, on: 'keep' })
		},
		keep: (id) => {
			setOpen(null)
			setRowFocus({ id, on: 'remove' })
		},
		confirm: async (id) => {
			const from = document.activeElement
			setFailure('')
			setPendingID(id)
			const result = await runDelete({ contactId, field: field.name, entryId: id })
			setPendingID('')
			settle(id, from, result.error, {
				fallback: __('The entry could not be removed.', 'alphone-fields'),
				done: () => {
					toaster.show(__('Entry removed.', 'alphone-fields'))
					leave(id, from)
				},
				refused: () => {
					setOpen(null)
					whenFocusStayed(from, () => setRowFocus({ id, on: 'remove' }))
				},
			})
		},
	}

	const rowOf = (id: string): RowState => ({
		mode: open?.id === id ? open.as : 'show',
		pending: pendingID === id,
		locked: pendingID !== '' || settling.includes(id),
		focus: rowFocus?.id === id ? rowFocus : undefined,
	})

	return { failure, addFocus, added, on, rowOf, listed, loadMore }
}
