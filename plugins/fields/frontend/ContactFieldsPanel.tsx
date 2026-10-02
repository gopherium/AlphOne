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
	useToaster,
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
 * Renders the fields in the order the server lists them: a form per run of plain fields, a card per repeater.
 * @param props - The contact, its fields, the values the graph answered and the report of a repeater gone.
 * @returns The forms and the repeater cards.
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
	const edits = useFieldEdits(contactId, fields)
	return (
		<>
			{fieldRuns(fields).map((run) => {
				const [first] = run
				return first.kind === 'REPEATER' ? (
					<RepeaterEntries
						key={first.id}
						contactId={contactId}
						field={first}
						stored={stored[first.name]}
						onGone={onGone}
					/>
				) : (
					<FieldsForm key={first.id} fields={run} stored={stored} edits={edits} />
				)
			})}
		</>
	)
}

/**
 * Splits the fields, kept in order, into runs: each repeater alone and each stretch of plain fields together.
 * @param fields - The fields in the order the server lists them.
 * @returns The runs, each holding at least one field.
 */
function fieldRuns(fields: FieldRow[]) {
	const runs: FieldRow[][] = []
	for (const field of fields) {
		const last = runs.at(-1)
		if (last && field.kind !== 'REPEATER' && last[0].kind !== 'REPEATER') {
			last.push(field)
		} else {
			runs.push([field])
		}
	}
	return runs
}

/**
 * Holds the unsaved edits of every plain field of one contact, and the save that writes them all at once.
 * @param contactId - The contact the values belong to.
 * @param fields - The fields of the contact.
 * @returns The edits, what records one, the save, the run pressed last, whether it runs and the last failure.
 */
function useFieldEdits(contactId: string, fields: FieldRow[]) {
	const [edited, setEdited] = useState<Record<string, string>>({})
	const [pressedRun, setPressedRun] = useState('')
	const [written, write] = useGraphMutation(writeContactFieldsMutation)
	const graph = useGraph()
	const toaster = useToaster()
	const save = (run: string) => {
		setPressedRun(run)
		void write({ contactId, values: writable(fields, edited) }).then((result) => {
			if (!result.error) {
				setEdited((held) => unsent(held, edited))
				toaster.show(__('Fields saved.', 'alphone-fields'))
				graph.refetch([valuesOperation])
			}
		})
	}
	return {
		edited,
		edit: (name: string, text: string) => setEdited((held) => ({ ...held, [name]: text })),
		save,
		pressedRun,
		saving: written.fetching,
		failure: written.error,
	}
}

/**
 * Returns the edits made or changed since a write was sent.
 * @param held - The edits on screen when the write is answered.
 * @param sent - The edits on screen when the write was sent.
 * @returns The edits still unsaved.
 */
function unsent(held: Record<string, string>, sent: Record<string, string>) {
	return Object.fromEntries(Object.entries(held).filter(([name, text]) => sent[name] !== text))
}

/** FieldEdits are the unsaved edits every form of a contact shares, and the save that writes them. */
type FieldEdits = ReturnType<typeof useFieldEdits>

/**
 * Renders the form editing one run of a contact's plain fields, its Save fields on once the run is edited.
 * @param props - The run of plain fields, the values the graph answered and the edits every form shares.
 * @returns The value form, with the notice of a failed save pressed in it.
 */
function FieldsForm({
	fields,
	stored,
	edits,
}: {
	fields: FieldRow[]
	stored: Record<string, unknown>
	edits: FieldEdits
}) {
	const changed = fields.some((field) => field.name in edits.edited)
	const pressed = fields.some((field) => field.id === edits.pressedRun)
	return (
		<form
			className="godmin-form"
			onSubmit={(event) => {
				event.preventDefault()
				edits.save(fields[0].id)
			}}
		>
			{fields.map((field) => (
				<FieldInput
					key={field.id}
					field={field}
					value={edits.edited[field.name] ?? textOf(stored[field.name])}
					onChange={(next) => edits.edit(field.name, next)}
				/>
			))}
			{edits.failure && pressed ? (
				<ErrorNotice>
					{validationMessage(graphError(edits.failure), __('The fields could not be saved.', 'alphone-fields'))}
				</ErrorNotice>
			) : null}
			<Button type="submit" disabled={!changed || edits.saving} loading={edits.saving && pressed}>
				{__('Save fields', 'alphone-fields')}
			</Button>
		</form>
	)
}

/**
 * Returns the edited values in the form the write mutation takes.
 * @param fields - The fields of the contact.
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
