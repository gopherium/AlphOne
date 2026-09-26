// SPDX-License-Identifier: AGPL-3.0-or-later

import {
	__,
	Button,
	ErrorNotice,
	LoadingRows,
	RepeatRows,
	Stack,
	Text,
	graphError,
	sprintf,
	useGraph,
	useGraphMutation,
	useGraphQuery,
	validationMessage,
} from '@alphone/frontend-sdk'
import { useId, useMemo, useState } from 'react'

import { entryText, textOf, typedValue } from './cellText'
import type { EntryText, SubFieldRow } from './cellText'
import { FieldInput } from './cells'
import { contactValuesDocument } from './document'
import { fieldsQuery, writeContactFieldsMutation } from './operations'

const valuesOperation = 'ContactFieldValues'

/** FieldRow is one catalogue entry the panel renders an input for. */
interface FieldRow {
	id: string
	name: string
	label: string
	kind: string
	subFields: SubFieldRow[]
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
	return <FieldValues key={contactId} contactId={contactId} fields={fields} />
}

/**
 * Renders the value editor of one contact once its stored values load.
 * @param props - The contact and the fields it holds values for.
 * @returns The value editor, a loading placeholder or the failed read.
 */
function FieldValues({ contactId, fields }: { contactId: string; fields: FieldRow[] }) {
	const [values] = useGraphQuery({
		query: contactValuesDocument(fields.map((field) => field.name)),
		variables: { id: contactId },
	})
	let body = <LoadingRows label={__('Loading fields…', 'alphone-fields')} rows={fields.length} />
	if (values.error) {
		body = <ErrorNotice>{__('The fields could not be loaded.', 'alphone-fields')}</ErrorNotice>
	} else if (values.data) {
		const stored = (values.data.contact ?? {}) as Record<string, unknown>
		body = <FieldsForm contactId={contactId} fields={fields} stored={stored} />
	}

	return (
		<Stack direction="column" gap="sm">
			<Text variant="heading-sm" render={<h2 />}>
				{__('Fields', 'alphone-fields')}
			</Text>
			{body}
		</Stack>
	)
}

/**
 * Renders the form editing the stored values of one contact.
 * @param props - The contact, the fields it holds values for and the values the graph answered.
 * @returns The value form.
 */
function FieldsForm({
	contactId,
	fields,
	stored,
}: {
	contactId: string
	fields: FieldRow[]
	stored: Record<string, unknown>
}) {
	const [edited, setEdited] = useState<Record<string, string>>({})
	const [entries, setEntries] = useState<Record<string, EntryText[]>>({})
	const [written, write] = useGraphMutation(writeContactFieldsMutation)
	const graph = useGraph()

	return (
		<form
			className="godmin-form"
			onSubmit={(event) => {
				event.preventDefault()
				void write({ contactId, values: writable(fields, edited, entries) }).then((result) => {
					if (!result.error) {
						setEdited({})
						setEntries({})
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
			{fields.map((field) =>
				field.kind === 'REPEATER' ? (
					<EntriesInput
						key={field.id}
						field={field}
						stored={stored[field.name]}
						entries={entries[field.name]}
						onChange={(next) => setEntries((held) => ({ ...held, [field.name]: next }))}
					/>
				) : (
					<FieldInput
						key={field.id}
						field={field}
						value={edited[field.name] ?? textOf(stored[field.name])}
						onChange={(next) => setEdited({ ...edited, [field.name]: next })}
					/>
				),
			)}
			<Button type="submit" loading={written.fetching}>
				{__('Save fields', 'alphone-fields')}
			</Button>
		</form>
	)
}

/**
 * Renders the entries of one repeater, each a group of its sub field inputs.
 * @param props - The repeater, its stored entries, the edited ones and the change handler.
 * @returns The entries editor.
 */
function EntriesInput({
	field,
	stored,
	entries,
	onChange,
}: {
	field: FieldRow
	stored: unknown
	entries: EntryText[] | undefined
	onChange: (next: EntryText[]) => void
}) {
	const given = useMemo(() => entriesOf(field.subFields, stored), [field.subFields, stored])
	const heading = useId()

	return (
		<Stack direction="column" gap="sm" role="group" aria-labelledby={heading}>
			<Text variant="heading-sm" render={<h3 />} id={heading}>
				{field.label}
			</Text>
			<RepeatRows
				rows={entries ?? given}
				onChange={onChange}
				blank={() => entryText(field.subFields, {})}
				renderRow={(entry, update) =>
					field.subFields.map((column) => (
						<FieldInput
							key={column.name}
							field={column}
							value={entry[column.name]}
							onChange={(next) => update({ ...entry, [column.name]: next })}
						/>
					))
				}
				rowLabel={(at) =>
					sprintf(__('%(label)s %(number)d', 'alphone-fields'), { label: field.label, number: at + 1 })
				}
				labels={{
					add: sprintf(__('Add an entry to %(label)s', 'alphone-fields'), { label: field.label }),
					empty: __('No entries yet.', 'alphone-fields'),
					moveUp: __('Move entry up', 'alphone-fields'),
					moveDown: __('Move entry down', 'alphone-fields'),
					remove: __('Remove entry', 'alphone-fields'),
				}}
			/>
		</Stack>
	)
}

/**
 * Returns the text of every stored entry of one repeater.
 * @param subFields - The sub fields every entry holds.
 * @param stored - The entries the graph answered, absent when none are stored.
 * @returns The entries as text, in stored order.
 */
function entriesOf(subFields: SubFieldRow[], stored: unknown) {
	const held = (stored ?? []) as Record<string, unknown>[]
	return held.map((entry) => entryText(subFields, entry))
}

/**
 * Returns the edited values in the form the write mutation takes.
 * @param fields - The catalogue the panel renders.
 * @param edited - The text the operator typed, by field name.
 * @param entries - The entries the operator changed, by repeater name.
 * @returns The values keyed by field name.
 */
function writable(
	fields: FieldRow[],
	edited: Record<string, string>,
	entries: Record<string, EntryText[]>,
) {
	const values: Record<string, unknown> = {}
	for (const field of fields) {
		const value = writableValue(field, edited, entries)
		if (value !== undefined) {
			values[field.name] = value
		}
	}
	return values
}

/**
 * Returns the value one field sends.
 * @param field - The catalogue entry.
 * @param edited - The text the operator typed, by field name.
 * @param entries - The entries the operator changed, by repeater name.
 * @returns The typed value, or undefined when the operator left the field untouched.
 */
function writableValue(
	field: FieldRow,
	edited: Record<string, string>,
	entries: Record<string, EntryText[]>,
) {
	if (field.kind === 'REPEATER') {
		const changed = entries[field.name]
		return changed === undefined ? undefined : typedEntries(field.subFields, changed)
	}
	const text = edited[field.name]
	return text === undefined ? undefined : typedValue(field.kind, text)
}

/**
 * Returns the entries of one repeater in the form the write mutation takes.
 * @param subFields - The sub fields every entry holds.
 * @param changed - The entries as text, in order.
 * @returns The typed entries, or null when none are left.
 */
function typedEntries(subFields: SubFieldRow[], changed: EntryText[]) {
	if (changed.length === 0) {
		return null
	}
	return changed.map((entry) =>
		Object.fromEntries(subFields.map((column) => [column.name, typedValue(column.kind, entry[column.name])])),
	)
}
