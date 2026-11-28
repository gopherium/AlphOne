// SPDX-License-Identifier: AGPL-3.0-or-later

import { __, sprintf, useGraphMutation, useToaster, writeFailure } from '@alphone/frontend-sdk'
import { useLayoutEffect, useRef } from 'react'
import type { RefObject } from 'react'

import type { DefineFieldMutation, FieldKind } from './gql/graphql'
import { defineFieldMutation, orderFieldsMutation } from './operations'

/** Restorable is an archived field as the catalogue showed it, enough to define it again. */
export interface Restorable {
	id: string
	name: string
	label: string
	kind: string
	subFields: { name: string; label: string; kind: string }[]
}

/**
 * Returns the restore that defines an archived field again and puts it back between the neighbours it had.
 * @param rows - The rows in the order shown.
 * @param onChanged - The reload run after every answer a restore gets, except a define lost on the way.
 * @param archived - The shown ids whose archive worked, left out of every order until their define succeeds.
 * @param gone - The ids of every field the reader archived, kept in their places in the order the list remembers.
 * @returns The restore, given the field, answering whether the field came back, one restore at a time.
 */
export function useRestore(
	rows: readonly { id: string }[],
	onChanged: () => void,
	archived: RefObject<Set<string>>,
	gone: RefObject<ReadonlySet<string>>,
) {
	const toaster = useToaster()
	const [, define] = useGraphMutation(defineFieldMutation)
	const [, order] = useGraphMutation(orderFieldsMutation)
	const shown = useRef<readonly string[]>([])
	const remembered = useRef<readonly string[]>([])
	const back = useRef(new Set<string>())
	const queue = useRef(Promise.resolve())
	useLayoutEffect(() => {
		shown.current = rows.map((row) => row.id)
		remembered.current = keptInPlace(remembered.current, shown.current, gone.current)
		for (const id of shown.current) {
			back.current.delete(id)
		}
	})
	const restoreOne = async (field: Restorable) => {
		const defined = await define(definitionOf(field))
		if (defined.error) {
			toaster.show(writeFailure(__('Could not restore the field.', 'alphone-fields'), defined.error))
			if (!defined.error.networkError) {
				onChanged()
			}
			return false
		}
		toaster.show(sprintf(__('"%(label)s" has been restored.', 'alphone-fields'), { label: field.label }))
		archived.current.delete(field.id)
		back.current.add((defined.data as DefineFieldMutation).defineField.id)
		const live = new Set([...shown.current.filter((id) => !archived.current.has(id)), ...back.current])
		const ordered = await order({ ids: remembered.current.filter((id) => live.has(id)) })
		if (ordered.error) {
			toaster.show(writeFailure(__('Could not order the fields.', 'alphone-fields'), ordered.error))
		}
		onChanged()
		return true
	}
	return (field: Restorable) => {
		const restoring = queue.current.then(() => restoreOne(field))
		queue.current = restoring.then(settled)
		return restoring
	}
}

/** Takes the answer of a restore that finished, so the next one may start. */
function settled() {}

/**
 * Returns the define that brings a field back as it was, its sub fields without what the read added.
 * @param field - The field as the catalogue showed it.
 * @returns The define's variables.
 */
function definitionOf(field: Restorable) {
	return {
		name: field.name,
		label: field.label,
		kind: field.kind as FieldKind,
		subFields: field.subFields.map(({ name, label, kind }) => ({ name, label, kind: kind as FieldKind })),
	}
}

/**
 * Returns the ids shown with every archived id the list remembered put back right after the id it followed.
 * @param remembered - The order the list remembered, archived ids in their places.
 * @param shown - The ids in the order shown now.
 * @param gone - The ids the reader archived.
 * @returns The order to remember now.
 */
function keptInPlace(remembered: readonly string[], shown: readonly string[], gone: ReadonlySet<string>) {
	const next = [...shown]
	let after: string | undefined
	for (const id of remembered) {
		if (gone.has(id) && !next.includes(id)) {
			next.splice(after === undefined ? 0 : next.indexOf(after) + 1, 0, id)
		}
		if (next.includes(id)) {
			after = id
		}
	}
	return next
}
