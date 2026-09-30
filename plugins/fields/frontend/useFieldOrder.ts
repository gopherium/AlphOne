// SPDX-License-Identifier: AGPL-3.0-or-later

import { useGraphMutation } from '@alphone/frontend-sdk'
import type { GraphFailure } from '@alphone/frontend-sdk'
import { useLayoutEffect, useRef, useState } from 'react'

import { orderFieldsMutation } from './operations'

/** Ordered is one live field an order places, known by its id. */
interface Ordered {
	id: string
}

/** FieldOrder is the order the list shows, the move that changes it and the failure of the last order answered. */
interface FieldOrder<T> {
	rows: readonly T[]
	move: (at: number, offset: number) => void
	failure: GraphFailure | undefined
}

/**
 * Returns the live fields in the order to show, with the move that saves a new order one save at a time.
 * @param fields - The live fields in the order the server answered.
 * @param onAnswered - The reload run once the last save is answered, and after a refused one.
 * @returns The rows, the move and the failure of the last order answered.
 */
export function useFieldOrder<T extends Ordered>(fields: readonly T[], onAnswered: () => void): FieldOrder<T> {
	const [ordered, order] = useGraphMutation(orderFieldsMutation)
	const [local, setLocal] = useState<readonly string[] | null>(null)
	const [saving, setSaving] = useState(false)
	const inFlight = useRef(false)
	const [seen, setSeen] = useState(fields)
	if (seen !== fields) {
		setSeen(fields)
		if (!saving) {
			setLocal(null)
		}
	}
	const rows = arranged(fields, local)
	const shown = useRef<readonly string[]>([])
	useLayoutEffect(() => {
		shown.current = rows.map((row) => row.id)
	})

	const save = (ids: readonly string[], before: readonly string[] | null) => {
		inFlight.current = true
		setSaving(true)
		void order({ ids: [...ids] }).then((result) => {
			const now = shown.current
			if (!result.error && now.join(' ') !== ids.join(' ')) {
				save(now, ids)
				return
			}
			inFlight.current = false
			setSaving(false)
			if (result.error?.networkError) {
				setLocal(before)
				return
			}
			if (result.error) {
				setLocal(null)
			}
			onAnswered()
		})
	}
	const move = (at: number, offset: number) => {
		const next = moved(rows, at, offset).map((row) => row.id)
		setLocal(next)
		if (!inFlight.current) {
			save(next, local)
		}
	}
	return { rows, move, failure: ordered.error }
}

/**
 * Returns the live fields in the order the reader set, ids no longer live left out and new fields last.
 * @param fields - The live fields in the order the server answered.
 * @param local - The ids in the order the reader set, or null to follow the server.
 * @returns The rows to show.
 */
function arranged<T extends Ordered>(fields: readonly T[], local: readonly string[] | null): readonly T[] {
	if (local === null) {
		return fields
	}
	const placed = (field: T) => local.indexOf(field.id)
	const kept = fields.filter((field) => placed(field) >= 0).sort((one, other) => placed(one) - placed(other))
	return [...kept, ...fields.filter((field) => placed(field) < 0)]
}

/**
 * Returns a copy of the rows with one row moved by the given offset.
 * @param rows - The rows in the order shown.
 * @param at - The place of the row to move.
 * @param offset - How many places the row moves, negative for up.
 * @returns The rows in their new order.
 */
function moved<T>(rows: readonly T[], at: number, offset: number): T[] {
	const next = [...rows]
	const [row] = next.splice(at, 1)
	next.splice(at + offset, 0, row)
	return next
}
