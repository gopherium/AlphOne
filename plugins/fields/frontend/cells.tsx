// SPDX-License-Identifier: AGPL-3.0-or-later

import { Checkbox, InputControl, Stack, Text, TextareaControl } from '@alphone/frontend-sdk'

import type { EntryText, SubFieldRow } from './cellText'

/**
 * Renders one input per sub field of an entry.
 * @param props - The sub fields, the text of every cell and the change handler.
 * @returns The cell inputs.
 */
export function EntryCells({
	subFields,
	draft,
	onChange,
}: {
	subFields: SubFieldRow[]
	draft: EntryText
	onChange: (name: string, text: string) => void
}) {
	return subFields.map((column) => (
		<FieldInput
			key={column.name}
			field={column}
			value={draft[column.name]}
			onChange={(next) => onChange(column.name, next)}
		/>
	))
}

/**
 * Renders one field's input, matched to the kind its definition declares.
 * @param props - The field, its current text and the change handler.
 * @returns The field input.
 */
export function FieldInput({
	field,
	value,
	onChange,
}: {
	field: { label: string; kind: string }
	value: string
	onChange: (next: string) => void
}) {
	if (field.kind === 'BOOLEAN') {
		return (
			<Stack direction="row" gap="sm" align="center">
				<Checkbox
					aria-label={field.label}
					checked={value === 'true'}
					onCheckedChange={(checked) => onChange(checked ? 'true' : 'false')}
				/>
				<Text>{field.label}</Text>
			</Stack>
		)
	}
	if (field.kind === 'LONGTEXT') {
		return (
			<TextareaControl
				label={field.label}
				value={value}
				onChange={(event) => onChange(event.target.value)}
			/>
		)
	}
	return (
		<InputControl
			label={field.label}
			type={inputType(field.kind)}
			value={value}
			onChange={(event) => onChange(event.target.value)}
		/>
	)
}

/**
 * Returns the HTML input type one field kind is edited with.
 * @param kind - The kind the definition declares.
 * @returns The input type.
 */
function inputType(kind: string) {
	if (kind === 'NUMBER') {
		return 'number'
	}
	if (kind === 'DATE') {
		return 'date'
	}
	return 'text'
}
