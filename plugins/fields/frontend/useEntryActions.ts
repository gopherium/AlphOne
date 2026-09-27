// SPDX-License-Identifier: AGPL-3.0-or-later

import { __, useGraph, useGraphMutation } from '@alphone/frontend-sdk'
import type { GraphFailure } from '@alphone/frontend-sdk'
import { useState } from 'react'

import { neighbour } from './entries'
import type { RepeaterRow, StoredEntry } from './entries'
import { entryMessage, outcomeOf, refetchAfter } from './entryOutcome'
import { whenFocusStayed } from './focus'
import { deleteContactFieldEntryMutation } from './operations'

/** RowFocus names the button of one row focus moves to. */
export interface RowFocus {
	id: string
	on: 'first' | 'remove' | 'keep'
}

/** RowState is how one row shows and what it answers to. */
export interface RowState {
	mode: 'show' | 'confirm'
	pending: boolean
	locked: boolean
	focus: RowFocus | undefined
}

/** EntryHandlers are the actions the buttons of a row run. */
export interface EntryHandlers {
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
 * Holds the state the rows of one repeater share, and the actions that change it.
 * @param props - The contact, the repeater, the entries shown and the report of a repeater gone.
 * @returns The failure to show, the add form's focus count, the add hook, the actions and each row's state.
 */
export function useEntryActions({
	contactId,
	field,
	entries,
	onGone,
}: {
	contactId: string
	field: RepeaterRow
	entries: readonly StoredEntry[]
	onGone: GoneHandler
}) {
	const [open, setOpen] = useState<string>('')
	const [pendingID, setPendingID] = useState('')
	const [settling, setSettling] = useState<readonly string[]>([])
	const [failure, setFailure] = useState('')
	const [rowFocus, setRowFocus] = useState<RowFocus | null>(null)
	const [addFocus, setAddFocus] = useState(0)
	const [, runDelete] = useGraphMutation(deleteContactFieldEntryMutation)
	const graph = useGraph()

	const added = () => setAddFocus((count) => count + 1)

	const leave = (id: string, from: Element | null) => {
		const next = neighbour(entries, id, settling)
		setOpen('')
		setSettling((held) => [...held, id])
		whenFocusStayed(from, () => (next === '' ? added() : setRowFocus({ id: next, on: 'first' })))
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
		remove: (id) => {
			setOpen(id)
			setRowFocus({ id, on: 'keep' })
		},
		keep: (id) => {
			setOpen('')
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
				done: () => leave(id, from),
				refused: () => {
					setOpen('')
					whenFocusStayed(from, () => setRowFocus({ id, on: 'remove' }))
				},
			})
		},
	}

	const rowOf = (id: string): RowState => ({
		mode: open === id ? 'confirm' : 'show',
		pending: pendingID === id,
		locked: pendingID !== '' || settling.includes(id),
		focus: rowFocus?.id === id ? rowFocus : undefined,
	})

	return { failure, addFocus, added, on, rowOf }
}
