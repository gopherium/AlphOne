// SPDX-License-Identifier: AGPL-3.0-or-later

import {
	Button,
	IconButton,
	LogItem,
	LogTime,
	Stack,
	Text,
	__,
	pencil,
	sprintf,
	trash,
} from '@alphone/frontend-sdk'
import { useEffect, useRef, useState } from 'react'
import type { RefObject } from 'react'

import { entryText } from './cellText'
import type { EntryText, SubFieldRow } from './cellText'
import { EntryCells } from './cells'
import { ConfirmActions } from './ConfirmActions'
import { entryName, entryParts, isBlank } from './entries'
import type { StoredEntry } from './entries'
import { focusFirstControl } from './focus'
import type { EntryHandlers, RowState } from './useEntryActions'

/**
 * Renders one entry as a log item: its day, its text and a line per other cell, with its actions, or its editor.
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
	const editRef = useRef<HTMLButtonElement>(null)
	const removeRef = useRef<HTMLButtonElement>(null)
	const keepRef = useRef<HTMLButtonElement>(null)

	useEffect(() => {
		if (row.focus) {
			const target = { first: editRef, remove: removeRef, keep: keepRef }[row.focus.on]
			;(target.current as HTMLButtonElement).focus()
		}
	}, [row.focus])

	if (row.mode === 'edit') {
		return (
			<LogItem aria-label={name}>
				<EntryEditor
					subFields={subFields}
					cells={entry.cells}
					name={name}
					pending={row.pending}
					locked={row.locked}
					onSave={(draft) => void on.save(entry.id, draft)}
					onCancel={() => on.cancel(entry.id)}
				/>
			</LogItem>
		)
	}
	return (
		<LogItem
			aria-label={name}
			label={parts.day ? <LogTime dateTime={parts.day.at}>{parts.day.text}</LogTime> : undefined}
			actions={
				row.mode === 'confirm' ? (
					<ConfirmActions
						labels={{
							question: __('Remove this entry?', 'alphone-fields'),
							confirm: __('Remove', 'alphone-fields'),
						}}
						locked={row.locked}
						pending={row.pending}
						keepRef={keepRef}
						onConfirm={() => void on.confirm(entry.id)}
						onKeep={() => on.keep(entry.id)}
					/>
				) : (
					<RowActions
						name={name}
						locked={row.locked}
						editRef={editRef}
						removeRef={removeRef}
						onEdit={() => on.edit(entry.id)}
						onRemove={() => on.remove(entry.id)}
					/>
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
 * Renders a row's Edit entry and Remove entry buttons, each named after the entry.
 * @param props - The entry's name, whether the row is locked, the two buttons' refs and their actions.
 * @returns The buttons.
 */
function RowActions({
	name,
	locked,
	editRef,
	removeRef,
	onEdit,
	onRemove,
}: {
	name: string
	locked: boolean
	editRef: RefObject<HTMLButtonElement | null>
	removeRef: RefObject<HTMLButtonElement | null>
	onEdit: () => void
	onRemove: () => void
}) {
	return (
		<>
			<IconButton
				ref={editRef}
				icon={pencil}
				variant="minimal"
				tone="neutral"
				size="compact"
				disabled={locked}
				label={sprintf(__('Edit entry: %(name)s', 'alphone-fields'), { name })}
				onClick={onEdit}
			/>
			<IconButton
				ref={removeRef}
				icon={trash}
				variant="minimal"
				tone="neutral"
				size="compact"
				disabled={locked}
				label={sprintf(__('Remove entry: %(name)s', 'alphone-fields'), { name })}
				onClick={onRemove}
			/>
		</>
	)
}

/**
 * Renders the form editing one entry in place, its cells drafted from the stored entry.
 * @param props - The sub fields, the stored cells, the entry's name, the row's state and the two actions.
 * @returns The editor form.
 */
function EntryEditor({
	subFields,
	cells,
	name,
	pending,
	locked,
	onSave,
	onCancel,
}: {
	subFields: SubFieldRow[]
	cells: Record<string, unknown>
	name: string
	pending: boolean
	locked: boolean
	onSave: (draft: EntryText) => void
	onCancel: () => void
}) {
	const [draft, setDraft] = useState(() => entryText(subFields, cells))
	const formRef = useRef<HTMLFormElement>(null)

	useEffect(() => {
		focusFirstControl(formRef.current as HTMLFormElement)
	}, [])

	return (
		<form
			ref={formRef}
			className="godmin-form"
			aria-label={sprintf(__('Edit entry: %(name)s', 'alphone-fields'), { name })}
			onSubmit={(event) => {
				event.preventDefault()
				onSave(draft)
			}}
		>
			<EntryCells
				subFields={subFields}
				draft={draft}
				disabled={locked}
				onChange={(key, text) => setDraft((held) => ({ ...held, [key]: text }))}
			/>
			<Stack direction="row" gap="sm" align="center">
				<Button type="submit" size="compact" disabled={locked || isBlank(draft)} loading={pending}>
					{__('Save entry', 'alphone-fields')}
				</Button>
				<Button variant="minimal" tone="neutral" size="compact" disabled={locked} onClick={onCancel}>
					{__('Cancel', 'alphone-fields')}
				</Button>
			</Stack>
		</form>
	)
}
