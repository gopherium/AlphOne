// SPDX-License-Identifier: AGPL-3.0-or-later

import { __, useGraphMutation, useToaster, writeFailure } from '@alphone/frontend-sdk'
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
 * Returns the restore that defines an archived field again and puts it back at its place among the live rows shown.
 * @param rows - The rows in the order shown.
 * @param onChanged - The reload run after every answer a restore gets, except a define lost on the way.
 * @param archived - The ids of the rows still shown whose archive worked, which the order leaves out.
 * @returns The restore, given the field and the place it held, answering whether the field came back.
 */
export function useRestore(
	rows: readonly { id: string }[],
	onChanged: () => void,
	archived: RefObject<ReadonlySet<string>>,
) {
	const toaster = useToaster()
	const [, define] = useGraphMutation(defineFieldMutation)
	const [, order] = useGraphMutation(orderFieldsMutation)
	const shown = useRef<readonly string[]>([])
	useLayoutEffect(() => {
		shown.current = rows.map((row) => row.id)
	})
	return async (field: Restorable, at: number) => {
		const defined = await define(definitionOf(field))
		if (defined.error) {
			toaster.show(writeFailure(__('Could not restore the field.', 'alphone-fields'), defined.error))
			if (!defined.error.networkError) {
				onChanged()
			}
			return false
		}
		toaster.show(__('Field restored.', 'alphone-fields'))
		const id = (defined.data as DefineFieldMutation).defineField.id
		const live = shown.current.filter((shownId) => !archived.current.has(shownId))
		const ordered = await order({ ids: placed(live, id, at) })
		if (ordered.error) {
			toaster.show(writeFailure(__('Could not order the fields.', 'alphone-fields'), ordered.error))
		}
		onChanged()
		return true
	}
}

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
 * Returns the ids shown with one id placed at the given place, last when fewer rows are shown now.
 * @param ids - The ids in the order shown.
 * @param id - The id to place.
 * @param at - The place it held.
 * @returns The ids in their new order.
 */
function placed(ids: readonly string[], id: string, at: number) {
	const next = ids.filter((shown) => shown !== id)
	next.splice(at, 0, id)
	return next
}
