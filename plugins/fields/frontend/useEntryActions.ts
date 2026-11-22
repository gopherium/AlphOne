// SPDX-License-Identifier: AGPL-3.0-or-later

import { __, dialogWrite, useGraph, useGraphMutation, useToaster } from '@alphone/frontend-sdk'
import type { DialogAnswer, GraphFailure } from '@alphone/frontend-sdk'
import { useState } from 'react'

import type { EntryText } from './cellText'
import { neighbour, typedEntry } from './entries'
import type { RepeaterRow, StoredEntry } from './entries'
import { entryFailure, entryMessage, outcomeOf, refetchAfter } from './entryOutcome'
import { focusedConfirm, whenFocusStayed } from './focus'
import { deleteContactFieldEntryMutation, updateContactFieldEntryMutation } from './operations'

/** RowFocus names the row whose first button focus moves to. */
export interface RowFocus {
	id: string
}

/** RowMode is how one row shows. */
type RowMode = 'show' | 'edit'

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
}

/** Removal is the confirm asking before one entry is removed: its write and its close. */
export interface Removal {
	confirm: (signal: AbortSignal) => Promise<DialogAnswer | undefined>
	close: () => void
}

/** GoneHandler reports a repeater the catalogue no longer holds, and where focus was when the call began. */
export type GoneHandler = (message: string, from: Element | null) => void

/** SettleSteps are what a settled call says it failed with and does once it succeeds. */
interface SettleSteps {
	failed: string
	done: () => void
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
 * @returns The add focus count, the add hook, the actions, each row's state, the rows, Load more and the removal asked.
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
	const [editing, setEditing] = useState('')
	const [asking, setAsking] = useState('')
	const [pendingID, setPendingID] = useState('')
	const [settling, setSettling] = useState<readonly string[]>([])
	const [rowFocus, setRowFocus] = useState<RowFocus | null>(null)
	const [addFocus, setAddFocus] = useState(0)
	const { listed, loadMore } = useDrawnEntries(entries, firstPage, [editing, asking, rowFocus?.id])
	const [, runUpdate] = useGraphMutation(updateContactFieldEntryMutation)
	const graph = useGraph()
	const toaster = useToaster()

	const focusAdd = () => setAddFocus((count) => count + 1)

	const added = (from: Element | null) => {
		whenFocusStayed(from, focusAdd)
	}

	const leave = (id: string, from: Element | null) => {
		const next = neighbour(listed, id, settling)
		setEditing('')
		setSettling((held) => [...held, id])
		whenFocusStayed(from, () => (next === '' ? focusAdd() : setRowFocus({ id: next })))
	}

	const settle = (
		id: string,
		from: Element | null,
		error: GraphFailure | undefined,
		steps: SettleSteps,
	): string | undefined => {
		refetchAfter(graph, error)
		const outcome = outcomeOf(error)
		if (outcome === 'done') {
			steps.done()
			return undefined
		}
		if (outcome === 'field-gone') {
			onGone(entryMessage(error as GraphFailure, steps.failed), from)
			return undefined
		}
		const words = entryFailure(steps.failed, error as GraphFailure)
		if (outcome === 'entry-gone') {
			toaster.show(words)
			leave(id, from)
			return undefined
		}
		return words
	}

	const on: EntryHandlers = {
		edit: (id) => {
			setEditing(id)
		},
		cancel: (id) => {
			setEditing('')
			setRowFocus({ id })
		},
		save: async (id, draft) => {
			const from = document.activeElement
			setPendingID(id)
			const result = await runUpdate({
				contactId,
				field: field.name,
				entryId: id,
				entry: typedEntry(field.subFields, draft),
			})
			setPendingID('')
			const rejected = settle(id, from, result.error, {
				failed: __('Could not save the entry.', 'alphone-fields'),
				done: () => {
					setEditing('')
					toaster.show(__('Entry saved.', 'alphone-fields'))
					whenFocusStayed(from, () => setRowFocus({ id }))
				},
			})
			if (rejected !== undefined) {
				toaster.show(rejected)
			}
		},
		remove: (id) => {
			setEditing('')
			setAsking(id)
		},
	}

	const removal: Removal = {
		confirm: async (signal) => {
			const id = asking
			const from = focusedConfirm()
			setPendingID(id)
			const variables = { contactId, field: field.name, entryId: id }
			const writing = dialogWrite(graph.client, deleteContactFieldEntryMutation, variables, signal)
			const result = await writing.finally(() => setPendingID(''))
			const rejected = settle(id, from, result.error, {
				failed: __('Could not remove the entry.', 'alphone-fields'),
				done: () => {
					toaster.show(__('Entry removed.', 'alphone-fields'))
					leave(id, from)
				},
			})
			return rejected === undefined ? undefined : { error: rejected }
		},
		close: () => setAsking(''),
	}

	const rowOf = (id: string): RowState => ({
		mode: editing === id ? 'edit' : 'show',
		pending: pendingID === id,
		locked: pendingID !== '' || settling.includes(id),
		focus: rowFocus?.id === id ? rowFocus : undefined,
	})

	return { addFocus, added, on, rowOf, listed, loadMore, removal: asking === '' ? undefined : removal }
}
