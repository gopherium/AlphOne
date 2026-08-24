// SPDX-License-Identifier: AGPL-3.0-or-later

import {
	__,
	Button,
	Checkbox,
	ErrorNotice,
	InputControl,
	Stack,
	Text,
	graphError,
	useGraph,
	useGraphMutation,
	useGraphQuery,
	validationMessage,
} from '@alphone/frontend-sdk'
import { useState } from 'react'

import { contactValuesDocument } from './document'
import { fieldsQuery, writeContactFieldsMutation } from './operations'

const valuesOperation = 'ContactFieldValues'

/** FieldRow is one catalogue entry the panel renders an input for. */
interface FieldRow {
	id: string
	name: string
	label: string
	kind: string
}

/**
 * Renders the runtime defined fields of one contact.
 * @param props - The contact whose values the panel reads and writes.
 * @returns The contact fields panel.
 */
export function ContactFieldsPanel({ contactId }: { contactId: string }) {
	const [catalogue] = useGraphQuery({ query: fieldsQuery })
	const fields = (catalogue.data?.fields ?? []) as FieldRow[]

	if (catalogue.error || fields.length === 0) {
		return null
	}
	return <FieldValues contactId={contactId} fields={fields} />
}

/**
 * Renders the value editor for the given fields of one contact.
 * @param props - The contact and the fields it holds values for.
 * @returns The value editor.
 */
function FieldValues({ contactId, fields }: { contactId: string; fields: FieldRow[] }) {
	const [values] = useGraphQuery({
		query: contactValuesDocument(fields.map((field) => field.name)),
		variables: { id: contactId },
	})
	const [edited, setEdited] = useState<Record<string, string>>({})
	const [written, write] = useGraphMutation(writeContactFieldsMutation)
	const graph = useGraph()
	const stored = (values.data?.contact ?? {}) as Record<string, unknown>

	return (
		<Stack direction="column" gap="sm">
			<Text variant="heading-sm" render={<h2 />}>
				{__('Fields', 'alphone-fields')}
			</Text>
			<form
				className="godmin-form"
				onSubmit={(event) => {
					event.preventDefault()
					void write({ contactId, values: writable(fields, edited) }).then((result) => {
						if (!result.error) {
							setEdited({})
							graph.refetch([valuesOperation])
						}
					})
				}}
			>
				{written.error ? (
					<ErrorNotice>
						{validationMessage(graphError(written.error), __('The fields could not be saved.', 'alphone-fields'))}
					</ErrorNotice>
				) : null}
				{fields.map((field) => (
					<FieldInput
						key={field.id}
						field={field}
						value={edited[field.name] ?? textOf(stored[field.name])}
						onChange={(next) => setEdited({ ...edited, [field.name]: next })}
					/>
				))}
				<Button type="submit" loading={written.fetching}>
					{__('Save fields', 'alphone-fields')}
				</Button>
			</form>
		</Stack>
	)
}

/**
 * Renders one field's input, matched to the kind its definition declares.
 * @param props - The field, its current text and the change handler.
 * @returns The field input.
 */
function FieldInput({
	field,
	value,
	onChange,
}: {
	field: FieldRow
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

/**
 * Returns the text form of a stored value.
 * @param stored - The value the graph answered.
 * @returns The text the input renders.
 */
function textOf(stored: unknown) {
	if (stored === null || stored === undefined) {
		return ''
	}
	return String(stored)
}

/**
 * Returns the edited values in the form the write mutation takes.
 * @param fields - The catalogue the panel renders.
 * @param edited - The text the operator typed, by field name.
 * @returns The values keyed by field name.
 */
function writable(fields: FieldRow[], edited: Record<string, string>) {
	const values: Record<string, unknown> = {}
	for (const field of fields) {
		const text = edited[field.name]
		if (text === undefined) {
			continue
		}
		values[field.name] = typedValue(field.kind, text)
	}
	return values
}

/**
 * Returns one typed value the graph accepts for the given kind.
 * @param kind - The kind the definition declares.
 * @param text - The text the operator typed.
 * @returns The typed value, or null when the text is blank.
 */
function typedValue(kind: string, text: string) {
	if (text === '') {
		return null
	}
	if (kind === 'NUMBER') {
		return Number(text)
	}
	if (kind === 'BOOLEAN') {
		return text === 'true'
	}
	return text
}
