// SPDX-License-Identifier: AGPL-3.0-or-later

import { Button, LogItem, LogTime, Stack, Text, __, sprintf } from '@alphone/frontend-sdk'
import { useEffect, useId, useRef } from 'react'
import type { RefObject } from 'react'

import type { SubFieldRow } from './cellText'
import { entryName, entryParts } from './entries'
import type { StoredEntry } from './entries'
import type { EntryHandlers, RowState } from './useEntryActions'

/**
 * Renders one entry as a log item: its day, its text and a line per other cell, with its actions.
 * @param props - The entry, the sub fields it holds, how the row shows and the actions its buttons run.
 * @returns The log item.
 */
export function EntryRow({
	entry,
	subFields,
	row,
	on,
}: {
	entry: StoredEntry
	subFields: SubFieldRow[]
	row: RowState
	on: EntryHandlers
}) {
	const parts = entryParts(subFields, entry.cells)
	const name = entryName(parts)
	const removeRef = useRef<HTMLButtonElement>(null)
	const keepRef = useRef<HTMLButtonElement>(null)

	useEffect(() => {
		if (row.focus) {
			const target = row.focus.on === 'keep' ? keepRef : removeRef
			;(target.current as HTMLButtonElement).focus()
		}
	}, [row.focus])

	return (
		<LogItem
			aria-label={name}
			label={parts.day ? <LogTime dateTime={parts.day.at}>{parts.day.text}</LogTime> : undefined}
			actions={
				row.mode === 'confirm' ? (
					<ConfirmActions
						row={row}
						keepRef={keepRef}
						onConfirm={() => void on.confirm(entry.id)}
						onKeep={() => on.keep(entry.id)}
					/>
				) : (
					<Button
						ref={removeRef}
						variant="minimal"
						tone="neutral"
						size="compact"
						disabled={row.locked}
						aria-label={sprintf(__('Remove entry: %(name)s', 'alphone-fields'), { name })}
						onClick={() => on.remove(entry.id)}
					>
						{__('Remove entry', 'alphone-fields')}
					</Button>
				)
			}
			body={parts.body}
		>
			{parts.lines.map((line) => (
				<Text key={line.key} variant="body-sm" render={<p />}>
					{line.text}
				</Text>
			))}
		</LogItem>
	)
}

/**
 * Renders the question that confirms a removal, with its Remove and Keep buttons.
 * @param props - How the row shows, the Keep button's ref and the two actions.
 * @returns The confirm group.
 */
function ConfirmActions({
	row,
	keepRef,
	onConfirm,
	onKeep,
}: {
	row: RowState
	keepRef: RefObject<HTMLButtonElement | null>
	onConfirm: () => void
	onKeep: () => void
}) {
	const question = useId()
	return (
		<Stack direction="row" gap="xs" align="center" wrap="wrap" role="group" aria-labelledby={question}>
			<Text id={question} variant="body-sm">
				{__('Remove this entry?', 'alphone-fields')}
			</Text>
			<Button
				variant="minimal"
				tone="neutral"
				size="compact"
				disabled={row.locked}
				loading={row.pending}
				onClick={onConfirm}
			>
				{__('Remove', 'alphone-fields')}
			</Button>
			<Button ref={keepRef} variant="minimal" tone="neutral" size="compact" disabled={row.locked} onClick={onKeep}>
				{__('Keep', 'alphone-fields')}
			</Button>
		</Stack>
	)
}
