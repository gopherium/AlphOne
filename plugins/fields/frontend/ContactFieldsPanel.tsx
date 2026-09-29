// SPDX-License-Identifier: AGPL-3.0-or-later

import {
	__,
	Button,
	ErrorNotice,
	LoadingRows,
	SectionTitle,
	Stack,
	graphError,
	useGraph,
	useGraphMutation,
	useGraphQuery,
	validationMessage,
} from '@alphone/frontend-sdk'
import { useId, useState } from 'react'

import { textOf, typedValue } from './cellText'
import type { SubFieldRow } from './cellText'
import { FieldInput } from './cells'
import { contactValuesDocument, valuesOperation } from './document'
import { whenFocusStayed } from './focus'
import { fieldsQuery, writeContactFieldsMutation } from './operations'
import { RepeaterEntries } from './RepeaterEntries'
import type { GoneHandler } from './useEntryActions'

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
 * Renders the fields of one contact once its stored values load, under a heading that takes focus when a repeater goes.
 * @param props - The contact and the fields it holds values for.
 * @returns The fields, a loading placeholder or the failed read.
 */
function FieldValues({ contactId, fields }: { contactId: string; fields: FieldRow[] }) {
	const [values] = useGraphQuery({
		query: contactValuesDocument(fields.map((field) => field.name)),
		variables: { id: contactId },
	})
	const [gone, setGone] = useState('')
	const heading = useId()
	const onGone: GoneHandler = (message, from) => {
		setGone(message)
		whenFocusStayed(from, () => (document.getElementById(heading) as HTMLElement).focus())
	}
	let body = <LoadingRows label={__('Loading fields…', 'alphone-fields')} rows={fields.length} />
	if (values.error) {
		body = <ErrorNotice>{__('The fields could not be loaded.', 'alphone-fields')}</ErrorNotice>
	} else if (values.data) {
		const stored = (values.data.contact ?? {}) as Record<string, unknown>
		body = <StoredFields contactId={contactId} fields={fields} stored={stored} onGone={onGone} />
	}

	return (
		<Stack direction="column" gap="sm">
			<SectionTitle id={heading} tabIndex={-1}>
				{__('Fields', 'alphone-fields')}
			</SectionTitle>
			{gone !== '' && <ErrorNotice>{gone}</ErrorNotice>}
			{body}
		</Stack>
	)
}

/**
 * Renders the form for the plain fields, then one section per repeater.
 * @param props - The contact, its fields, the values the graph answered and the report of a repeater gone.
 * @returns The plain fields form and the repeater sections.
 */
function StoredFields({
	contactId,
	fields,
	stored,
	onGone,
}: {
	contactId: string
	fields: FieldRow[]
	stored: Record<string, unknown>
	onGone: GoneHandler
}) {
	const others = fields.filter((field) => field.kind !== 'REPEATER')
	const repeaters = fields.filter((field) => field.kind === 'REPEATER')
	return (
		<>
			{others.length > 0 && <FieldsForm contactId={contactId} fields={others} stored={stored} />}
			{repeaters.map((field) => (
				<RepeaterEntries
					key={field.id}
					contactId={contactId}
					field={field}
					stored={stored[field.name]}
					onGone={onGone}
				/>
			))}
		</>
	)
}

/**
 * Renders the form editing the stored values of one contact's plain fields.
 * @param props - The contact, the plain fields it holds values for and the values the graph answered.
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
	const [written, write] = useGraphMutation(writeContactFieldsMutation)
	const graph = useGraph()

	return (
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
	)
}

/**
 * Returns the edited values in the form the write mutation takes.
 * @param fields - The plain fields the form renders.
 * @param edited - The text the operator typed, by field name.
 * @returns The values keyed by field name, the untouched fields left out.
 */
function writable(fields: FieldRow[], edited: Record<string, string>) {
	const values: Record<string, unknown> = {}
	for (const field of fields) {
		const text = edited[field.name]
		if (text !== undefined) {
			values[field.name] = typedValue(field.kind, text)
		}
	}
	return values
}
