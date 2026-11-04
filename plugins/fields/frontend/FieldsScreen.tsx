// SPDX-License-Identifier: AGPL-3.0-or-later

import {
	Badge,
	Button,
	ErrorNotice,
	InputControl,
	ListEmpty,
	LoadingScreen,
	PageScreen,
	RepeatRows,
	RowControls,
	SectionTitle,
	SelectControl,
	Stack,
	Text,
	ValidatedInputControl,
	__,
	formatList,
	formatNumber,
	keyFromLabel,
	list,
	sprintf,
	useGraph,
	useGraphMutation,
	useGraphQuery,
	useLastAnswer,
	useRefusedField,
	useToaster,
	writeFailure,
} from '@alphone/frontend-sdk'
import type { GraphFailure } from '@alphone/frontend-sdk'
import { useCallback, useEffect, useLayoutEffect, useRef, useState } from 'react'
import type { RefObject } from 'react'

import { ConfirmActions } from './ConfirmActions'
import { reasonOf } from './entryOutcome'
import { whenFocusStayed } from './focus'
import type { FieldCatalogueQuery, FieldKind } from './gql/graphql'
import { ENTRY_ID_KEY, kindItems, kindOf, subKindItems } from './kind'
import {
	archiveFieldMutation,
	defineFieldMutation,
	fieldCatalogueOperation,
	fieldCatalogueQuery,
} from './operations'
import { useFieldOrder } from './useFieldOrder'

/** FieldRow is one catalogue entry as the screen renders it. */
interface FieldRow {
	id: string
	name: string
	label: string
	kind: string
	subFields: SubFieldRow[]
}

/** SubFieldRow is one sub field of a repeater as the screen renders it. */
interface SubFieldRow {
	name: string
	label: string
	kind: string
}

/** DraftSubField is one sub field the add form holds before it is named. */
interface DraftSubField {
	label: string
	kind: FieldKind
}

/** KnownNames are the live fields, every stored field and the names the server refuses. */
interface KnownNames {
	live: FieldRow[]
	every: FieldRow[]
	reserved: string[]
}

/** FocusMark is the place of a row that held focus as it went and of the control in it that did. */
interface FocusMark {
	at: number
	control: number
}

/** TrashFocus names the row and the button in it focus moves to: its trash, or Keep in the question it asks. */
interface TrashFocus {
	id: string
	on: 'trash' | 'keep'
}

/** ArchiveQuestion is the row asking before it archives, whether its archive is under way and where focus sat. */
interface ArchiveQuestion {
	id: string
	locked: boolean
	from: Element | null
}

/** ArchiveRow is how one row's trash shows: asking, locked while an archive is under way, and the focus to take. */
interface ArchiveRow {
	asking: boolean
	locked: boolean
	focus: TrashFocus | undefined
}

/** ArchiveHandlers are the actions the trash and the question of a row run. */
interface ArchiveHandlers {
	ask: (id: string) => void
	keep: (id: string) => void
	confirm: (id: string) => void
}

/** Archiving is what the rows read and call to archive their fields. */
interface Archiving {
	rowOf: (id: string) => ArchiveRow
	on: ArchiveHandlers
}

/** TRASH is the place of the trash among the buttons of a row's controls. */
const TRASH = 2

/** RACED are the reasons a define answers when another field took its name first. */
const RACED = new Set(['field_name_taken', 'field_kind_locked'])

/** LABEL_REASONS are the reasons a define answers about the label it was sent. */
const LABEL_REASONS = new Set(['field_label_required', 'field_label_too_long'])

/**
 * Renders the catalogue of contact fields an operator defines.
 * @returns The fields screen.
 */
export function FieldsScreen() {
	const [catalogue] = useGraphQuery({ query: fieldCatalogueQuery, requestPolicy: 'cache-and-network' })
	const answer = useLastAnswer(catalogue.data)
	const reload = useCatalogueRefresh()
	const failed = <ErrorNotice>{__('The fields could not be loaded.', 'alphone-fields')}</ErrorNotice>

	if (catalogue.fetching && !answer) {
		return (
			<PageScreen title={__('Fields', 'alphone-fields')} headingLevel={2}>
				<LoadingScreen label={__('Loading fields…', 'alphone-fields')} />
			</PageScreen>
		)
	}
	if (catalogue.error && !answer) {
		return (
			<PageScreen title={__('Fields', 'alphone-fields')} headingLevel={2}>
				{failed}
			</PageScreen>
		)
	}
	const rows = catalogueRows(answer)
	return (
		<PageScreen title={__('Fields', 'alphone-fields')} headingLevel={2}>
			<Stack direction="column" gap="lg">
				{catalogue.error ? failed : null}
				<FieldList fields={rows.live} onChanged={reload} readFailure={catalogue.error} />
				<Stack direction="column" gap="sm">
					<SectionTitle>{__('Add a field', 'alphone-fields')}</SectionTitle>
					<AddFieldForm known={rows} onAnswered={reload} />
				</Stack>
			</Stack>
		</PageScreen>
	)
}

/**
 * Returns the live fields, every stored field and the reserved names a catalogue answer holds.
 * @param data - The catalogue answer, absent while none has arrived.
 * @returns The rows, each list empty when the answer lacks it.
 */
function catalogueRows(data: FieldCatalogueQuery | undefined): KnownNames {
	return {
		live: (data?.fields ?? []) as FieldRow[],
		every: (data?.every ?? []) as FieldRow[],
		reserved: data?.reservedFieldNames ?? [],
	}
}

/**
 * Returns the refresh rerunning the catalogue query against the network.
 * @returns The refresh callback.
 */
function useCatalogueRefresh() {
	const graph = useGraph()
	return () => {
		graph.refetch([fieldCatalogueOperation])
	}
}

/**
 * Renders the live fields in the order the reader set, each row moving and archiving its field, or a placeholder.
 * @param props - The live fields in the server's order, the reload after an archive or an order, the read's failure.
 * @returns The field list.
 */
function FieldList({
	fields,
	onChanged,
	readFailure,
}: {
	fields: FieldRow[]
	onChanged: () => void
	readFailure: GraphFailure | undefined
}) {
	const order = useFieldOrder(fields, onChanged)
	const archiving = useArchive(order.rows, onChanged, readFailure)
	const region = useRef<HTMLDivElement>(null)
	const body = useRef<HTMLTableSectionElement>(null)
	const onLeave = useFocusKept(region, body, order.rows)
	const empty = order.rows.length === 0

	return (
		<div
			ref={region}
			className="godmin-table-scroll"
			role="region"
			aria-label={__('Fields', 'alphone-fields')}
			tabIndex={empty ? -1 : 0}
		>
			{empty ? (
				<NoFields />
			) : (
				<FieldTable rows={order.rows} body={body} onMove={order.move} archiving={archiving} onLeave={onLeave} />
			)}
		</div>
	)
}

/**
 * Renders the placeholder of a catalogue holding no live field.
 * @returns The empty state.
 */
function NoFields() {
	return (
		<ListEmpty
			icon={list}
			title={__('No fields yet', 'alphone-fields')}
			description={__('Add a field to store more about every contact.', 'alphone-fields')}
		/>
	)
}

/**
 * Reports whether this render holds a read failure no earlier render held.
 * @param failure - The failure of the latest read.
 * @returns Whether the failure is new.
 */
function useNewFailure(failure: GraphFailure | undefined) {
	const [seen, setSeen] = useState(failure)
	if (seen !== failure) {
		setSeen(failure)
	}
	return Boolean(failure) && seen !== failure
}

/**
 * Holds the question the trash asks before an archive, one row at a time, and the archive its Archive button runs.
 * @param rows - The rows in the order shown.
 * @param onChanged - The reload run after every answer an archive gets.
 * @param readFailure - The latest read's failure, which frees a question waiting for that read.
 * @returns How each row's trash shows and the actions.
 */
function useArchive(rows: readonly FieldRow[], onChanged: () => void, readFailure: GraphFailure | undefined) {
	const toaster = useToaster()
	const [, archive] = useGraphMutation(archiveFieldMutation)
	const [question, setQuestion] = useState<ArchiveQuestion | null>(null)
	const [focus, setFocus] = useState<TrashFocus | null>(null)
	const readFailed = useNewFailure(readFailure)
	if (question !== null && !rows.some((row) => row.id === question.id)) {
		setQuestion(null)
	}
	if (focus !== null && !rows.some((row) => row.id === focus.id)) {
		setFocus(null)
	}
	if (readFailed && question?.locked) {
		setQuestion(null)
		whenFocusStayed(question.from, () => setFocus({ id: question.id, on: 'trash' }))
	}
	const on: ArchiveHandlers = {
		ask: (id) => {
			setQuestion({ id, locked: false, from: null })
			setFocus({ id, on: 'keep' })
		},
		keep: (id) => {
			setQuestion(null)
			setFocus({ id, on: 'trash' })
		},
		confirm: (id) => {
			const from = document.activeElement
			setQuestion({ id, locked: true, from })
			void archive({ id }).then((result) => {
				if (result.error) {
					setQuestion(null)
					toaster.show(writeFailure(__('Could not archive the field.', 'alphone-fields'), result.error))
					whenFocusStayed(from, () => setFocus({ id, on: 'trash' }))
				} else {
					toaster.show(__('Field archived.', 'alphone-fields'))
				}
				onChanged()
			})
		},
	}
	const rowOf = (id: string): ArchiveRow => ({
		asking: question?.id === id,
		locked: question?.locked === true,
		focus: focus?.id === id ? focus : undefined,
	})
	return { rowOf, on }
}

/**
 * Renders the table of the live fields, each row moving and archiving its field.
 * @param props - The rows in the order shown, the body ref and what the arrows, the trash and a leaving row call.
 * @returns The field table.
 */
function FieldTable({
	rows,
	body,
	onMove,
	archiving,
	onLeave,
}: {
	rows: readonly FieldRow[]
	body: RefObject<HTMLTableSectionElement | null>
	onMove: (at: number, offset: number) => void
	archiving: Archiving
	onLeave: (mark: FocusMark) => void
}) {
	return (
		<table className="godmin-table">
			<thead>
				<tr>
					<th scope="col">{__('Label', 'alphone-fields')}</th>
					<th scope="col">{__('Kind', 'alphone-fields')}</th>
					<th scope="col">{__('API name', 'alphone-fields')}</th>
					<th scope="col" className="godmin-table__actions" />
				</tr>
			</thead>
			<tbody ref={body}>
				{rows.map((field, at) => (
					<FieldTableRow
						key={field.id}
						field={field}
						at={at}
						count={rows.length}
						row={archiving.rowOf(field.id)}
						on={archiving.on}
						onMove={(offset) => onMove(at, offset)}
						onLeave={onLeave}
					/>
				))}
			</tbody>
		</table>
	)
}

/**
 * Keeps focus in the list as its rows change, and returns the report a row makes as it goes.
 * @param region - The region the list renders in.
 * @param body - The table body the rows render in, empty while no row is left.
 * @param rows - The rows in the order shown.
 * @returns The report of the control that held focus in a row as it went.
 */
function useFocusKept(
	region: RefObject<HTMLDivElement | null>,
	body: RefObject<HTMLTableSectionElement | null>,
	rows: readonly FieldRow[],
) {
	const gone = useRef<FocusMark | null>(null)
	const onLeave = useCallback((mark: FocusMark) => {
		gone.current = mark
	}, [])
	const shown = rows.map((row) => row.id).join(' ')
	const count = rows.length
	useLayoutEffect(() => {
		const lost = gone.current
		gone.current = null
		if (lost === null) {
			keepFocusInView(body)
			return
		}
		const row = body.current?.rows[Math.min(lost.at, count - 1)]
		const target = row?.querySelectorAll('button')[lost.control] ?? (region.current as HTMLDivElement)
		target.focus()
	}, [region, body, shown, count])
	return onLeave
}

/**
 * Scrolls the control holding focus in the table body into view.
 * @param body - The table body the rows render in, empty while no row is left.
 */
function keepFocusInView(body: RefObject<HTMLTableSectionElement | null>) {
	const focused = document.activeElement as Element
	if (body.current?.contains(focused)) {
		focused.scrollIntoView({ block: 'nearest' })
	}
}

/**
 * Reports where focus sat in a row as the row goes.
 * @param row - The row element.
 * @param onLeave - The report to make.
 */
function useLeaveReported(row: RefObject<HTMLTableRowElement | null>, onLeave: (mark: FocusMark) => void) {
	useLayoutEffect(() => {
		const element = row.current as HTMLTableRowElement
		return () => {
			const control = focusedControl(element)
			if (control >= 0) {
				onLeave({ at: element.sectionRowIndex, control })
			}
		}
	}, [row, onLeave])
}

/**
 * Returns the place among a row's controls of the one holding focus, a focus in its question counting as its trash.
 * @param row - The row element.
 * @returns The place, or -1 when focus sits elsewhere.
 */
function focusedControl(row: HTMLTableRowElement) {
	const focused = document.activeElement as HTMLElement
	if (row.querySelector('[role="group"]')?.contains(focused)) {
		return TRASH
	}
	return [...row.querySelectorAll('button')].indexOf(focused as HTMLButtonElement)
}

/**
 * Returns the trash of a row showing its controls.
 * @param row - The row element.
 * @returns The trash button.
 */
function trashOf(row: HTMLTableRowElement) {
	return row.querySelectorAll('button')[TRASH]
}

/**
 * Renders one live field beside the arrows that move it and the trash that asks to archive it, or the question.
 * @param props - The field, its place, the list's length, how its trash shows and what its buttons and its going call.
 * @returns The table row.
 */
function FieldTableRow({
	field,
	at,
	count,
	row,
	on,
	onMove,
	onLeave,
}: {
	field: FieldRow
	at: number
	count: number
	row: ArchiveRow
	on: ArchiveHandlers
	onMove: (offset: number) => void
	onLeave: (mark: FocusMark) => void
}) {
	const element = useRef<HTMLTableRowElement>(null)
	const keepRef = useRef<HTMLButtonElement>(null)
	useLeaveReported(element, onLeave)

	useEffect(() => {
		if (row.focus) {
			const target = row.focus.on === 'keep' ? keepRef.current : trashOf(element.current as HTMLTableRowElement)
			;(target as HTMLButtonElement).focus()
		}
	}, [row.focus])

	return (
		<tr ref={element}>
			<td>{field.label}</td>
			<td>
				<KindCell field={field} />
			</td>
			<td>
				<code>{field.name}</code>
			</td>
			<td className="godmin-table__actions">
				{row.asking ? (
					<ConfirmActions
						labels={{
							question: __('Archive this field?', 'alphone-fields'),
							confirm: __('Archive', 'alphone-fields'),
						}}
						locked={row.locked}
						pending={row.locked}
						keepRef={keepRef}
						onConfirm={() => on.confirm(field.id)}
						onKeep={() => on.keep(field.id)}
						layout="stacked"
					/>
				) : (
					<RowControls
						at={at}
						count={count}
						removable={!row.locked}
						labels={{
							moveUp: sprintf(__('Move %(label)s up', 'alphone-fields'), { label: field.label }),
							moveDown: sprintf(__('Move %(label)s down', 'alphone-fields'), { label: field.label }),
							remove: sprintf(__('Archive %(label)s', 'alphone-fields'), { label: field.label }),
						}}
						onMove={onMove}
						onRemove={() => on.ask(field.id)}
					/>
				)}
			</td>
		</tr>
	)
}

/**
 * Renders the kind of one field, with the labels of its sub fields when it holds any.
 * @param props - The catalogue row.
 * @returns The kind cell content.
 */
function KindCell({ field }: { field: FieldRow }) {
	const badge = <Badge intent="stable">{kindLabel(field.kind)}</Badge>
	if (field.subFields.length === 0) {
		return badge
	}
	return (
		<Stack direction="column" gap="xs" align="start">
			{badge}
			<Text variant="body-sm">{labelList(field.subFields)}</Text>
		</Stack>
	)
}

/**
 * Returns the human label of one field kind.
 * @param kind - The kind the definition declares.
 * @returns The label shown in the catalogue.
 */
function kindLabel(kind: string) {
	return kindOf({ value: kind }).label
}

/**
 * Returns the labels of the given sub fields as one list in the reader's language.
 * @param subFields - The sub fields of a repeater, in order.
 * @returns The joined labels.
 */
function labelList(subFields: SubFieldRow[]) {
	return formatList(subFields.map((column) => column.label))
}

/**
 * Returns the sub fields as the define mutation takes them, each named from its label.
 * @param drafts - The sub fields the operator listed, in order.
 * @returns The sub fields, each under a name no sibling repeats and entries do not keep their id under.
 */
function namedSubFields(drafts: DraftSubField[]) {
	const taken: string[] = [ENTRY_ID_KEY]
	return drafts.map((draft) => {
		const name = keyFromLabel(draft.label, { style: 'camel', taken })
		taken.push(name)
		return { name, label: draft.label, kind: draft.kind }
	})
}

/**
 * Returns the name a new field takes from its label.
 * @param label - The label of the new field.
 * @param known - Every stored field and the names the server refuses.
 * @param refused - The names the server refused as taken on this visit.
 * @returns The name, numbered past every name already taken.
 */
function fieldName(label: string, known: KnownNames, refused: readonly string[]) {
	const stored = known.every.map((row) => row.name)
	return keyFromLabel(label, { style: 'camel', taken: [...known.reserved, ...refused, ...stored] })
}

/**
 * Reports whether a live field already carries a label, ignoring case and outer spaces.
 * @param label - The label typed.
 * @param live - The live fields.
 * @returns True when a live field carries it.
 */
function labelHeld(label: string, live: FieldRow[]) {
	const typed = label.trim().toLowerCase()
	return live.some((row) => row.label.trim().toLowerCase() === typed)
}

/**
 * Reports whether a failed define is about the label the form holds, a repeater sending sub field labels too.
 * @param reason - The reason the define answered with.
 * @param repeater - Whether the define sent a repeater.
 * @returns Whether the message belongs under the label.
 */
function aboutFormLabel(reason: string, repeater: boolean) {
	return !repeater && LABEL_REASONS.has(reason)
}

/**
 * Renders the form defining one new field, naming it from its label.
 * @param props - The names a new field steps past and the reload run after every answer the server gives.
 * @returns The add field form.
 */
function AddFieldForm({ known, onAnswered }: { known: KnownNames; onAnswered: () => void }) {
	const toaster = useToaster()
	const [label, setLabel] = useState('')
	const [labelTaken, setLabelTaken] = useState(false)
	const [kind, setKind] = useState<FieldKind>('TEXT')
	const [subFields, setSubFields] = useState<DraftSubField[]>([])
	const [refused, setRefused] = useState<readonly string[]>([])
	const kinds = kindItems()
	const [defined, define] = useGraphMutation(defineFieldMutation)
	const form = useRef<HTMLFormElement>(null)
	const refusedLabel = useRefusedField(form, label)
	const repeater = kind === 'REPEATER'
	const submit = async () => {
		const taken = labelHeld(label, known.live)
		setLabelTaken(taken)
		if (taken) {
			return
		}
		const name = fieldName(label, known, refused)
		const sent = repeater ? namedSubFields(subFields) : undefined
		const result = await define({ name, label, kind, subFields: sent })
		if (!result.error?.networkError) {
			onAnswered()
		}
		if (!result.error) {
			setLabel('')
			setSubFields([])
			toaster.show(__('Field added.', 'alphone-fields'))
			return
		}
		const reason = reasonOf(result.error)
		if (RACED.has(reason)) {
			setRefused((held) => [...held, name])
		}
		if (!aboutFormLabel(reason, repeater) || !refusedLabel.refuse(result.error, label)) {
			toaster.show(writeFailure(__('Could not add the field.', 'alphone-fields'), result.error))
		}
	}

	return (
		<form
			ref={form}
			className="godmin-form godmin-form--inline"
			onSubmit={(event) => {
				event.preventDefault()
				void submit()
			}}
		>
			{labelTaken ? (
				<ErrorNotice>{__('A field with that label already exists.', 'alphone-fields')}</ErrorNotice>
			) : null}
			<div className="godmin-form__row">
				<div className="godmin-form__grow">
					<ValidatedInputControl
						label={__('Label', 'alphone-fields')}
						autoComplete="off"
						value={label}
						customValidity={refusedLabel.validity}
						onChange={(event) => {
							setLabel(event.target.value)
							setLabelTaken(false)
						}}
					/>
				</div>
				<SelectControl
					label={__('Kind', 'alphone-fields')}
					items={kinds}
					value={kinds.find((option) => option.value === kind)}
					onValueChange={(item) => setKind(kindOf(item).value)}
				/>
			</div>
			{repeater ? <SubFieldRows rows={subFields} onChange={setSubFields} /> : null}
			<Button type="submit" loading={defined.fetching}>
				{__('Add field', 'alphone-fields')}
			</Button>
		</form>
	)
}

/**
 * Renders the sub fields a new repeater holds, each with its label and kind.
 * @param props - The listed sub fields and what to call with a change.
 * @returns The sub fields editor.
 */
function SubFieldRows({
	rows,
	onChange,
}: {
	rows: DraftSubField[]
	onChange: (rows: DraftSubField[]) => void
}) {
	const kinds = subKindItems()

	return (
		<Stack direction="column" gap="sm">
			<SectionTitle level={3}>{__('Sub fields', 'alphone-fields')}</SectionTitle>
			<RepeatRows
				rows={rows}
				onChange={onChange}
				blank={(): DraftSubField => ({ label: '', kind: 'TEXT' })}
				renderRow={(row, update) => (
					<div className="godmin-form__row">
						<InputControl
							className="godmin-form__grow"
							label={__('Label', 'alphone-fields')}
							autoComplete="off"
							value={row.label}
							onChange={(event) => update({ ...row, label: event.target.value })}
						/>
						<SelectControl
							label={__('Kind', 'alphone-fields')}
							items={kinds}
							value={kinds.find((option) => option.value === row.kind)}
							onValueChange={(item) => update({ ...row, kind: kindOf(item).value })}
						/>
					</div>
				)}
				rowLabel={(at) => sprintf(__('Sub field %(number)s', 'alphone-fields'), { number: formatNumber(at + 1) })}
				labels={{
					add: __('Add sub field', 'alphone-fields'),
					empty: __('No sub fields yet.', 'alphone-fields'),
					moveUp: __('Move sub field up', 'alphone-fields'),
					moveDown: __('Move sub field down', 'alphone-fields'),
					remove: __('Remove sub field', 'alphone-fields'),
				}}
			/>
		</Stack>
	)
}
