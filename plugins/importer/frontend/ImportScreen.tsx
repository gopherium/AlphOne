// SPDX-License-Identifier: AGPL-3.0-or-later

import {
	Button,
	ErrorNotice,
	LoadingRows,
	LoadingScreen,
	PageScreen,
	SectionTitle,
	SelectControl,
	__,
	_x,
	formatNumber,
	graphError,
	sprintf,
	useGraph,
	useGraphMutation,
	useGraphQuery,
	useToaster,
	validationMessage,
} from '@alphone/frontend-sdk'
import type { GraphFailure } from '@alphone/frontend-sdk'
import { Suspense, lazy, useState } from 'react'

import { columnLabel } from './column'
import { fieldLabeller } from './fieldLabels'
import type { ImportDetailQuery } from './gql/graphql'
import {
	importCommitMutation,
	importDetailQuery,
	importSetMappingMutation,
} from './operations'

const RowsTable = lazy(() => import('./RowsTable'))

/** StoredImport is one import as the detail document selects it. */
export type StoredImport = NonNullable<ImportDetailQuery['importJob']>

/** ImportField is one target field as the detail document selects it. */
export type ImportField = ImportDetailQuery['importFields'][number]

// unmapped is the select value a column carries until a field is chosen.
const unmapped = 'not-imported'

/**
 * Renders one import: its column mapping, its staged rows, and the commit.
 * @returns The import screen.
 */
export function ImportScreen({ importId }: { importId: string }) {
	const [detail] = useGraphQuery({ query: importDetailQuery, variables: { id: importId } })

	if (detail.error) {
		return <ErrorNotice>{__('The import could not be loaded.', 'alphone-importer')}</ErrorNotice>
	}
	if (!detail.data) {
		return (
			<PageScreen title={_x('Import', 'admin section', 'alphone-importer')}>
				<LoadingScreen label={__('Loading import…', 'alphone-importer')} />
			</PageScreen>
		)
	}
	const { importJob: stored, importFields } = detail.data
	if (!stored) {
		return <ErrorNotice>{__('The import could not be loaded.', 'alphone-importer')}</ErrorNotice>
	}
	return (
		<PageScreen title={stored.filename}>
			<MappingForm stored={stored} fields={importFields} />
			<SectionTitle>{__('Rows', 'alphone-importer')}</SectionTitle>
			<Suspense fallback={<LoadingRows label={__('Loading the preview…', 'alphone-importer')} rows={3} />}>
				<RowsTable stored={stored} rows={stored.rows} fields={importFields} />
			</Suspense>
		</PageScreen>
	)
}

/**
 * Renders the column assignments and the control that commits them.
 * @returns The mapping form.
 */
function MappingForm({
	stored,
	fields,
}: {
	stored: StoredImport
	fields: readonly ImportField[]
}) {
	const graph = useGraph()
	const toaster = useToaster()
	const [assigned, setAssigned] = useState<Record<string, string>>(assignedOf(stored.mapping))
	const [save, startSave] = useGraphMutation(importSetMappingMutation)
	const [commit, startCommit] = useGraphMutation(importCommitMutation)
	const [failure, setFailure] = useState<string>()
	const refresh = () => graph.refetch(['ImportDetail', 'Imports'])
	const saveMapping = async () => {
		setFailure(undefined)
		const result = await startSave({ id: stored.id, assignments: assignmentsOf(assigned) })
		setFailure(failureOf(result.error, __('The mapping could not be saved.', 'alphone-importer')))
		if (result.data) {
			toaster.show(__('Mapping saved.', 'alphone-importer'))
		}
		refresh()
	}
	const commitImport = async () => {
		setFailure(undefined)
		const result = await startCommit({ id: stored.id })
		setFailure(failureOf(result.error, __('The import could not be committed.', 'alphone-importer')))
		if (result.data) {
			toaster.show(finishedMessage(result.data.importCommit))
		}
		refresh()
	}

	return (
		<form
			className="godmin-form"
			onSubmit={(event) => {
				event.preventDefault()
				void saveMapping()
			}}
		>
			{stored.columns.map((column, index) => (
				<ColumnSelect
					key={index}
					column={column}
					index={index}
					fields={fields}
					chosen={assigned[String(index)] ?? unmapped}
					onChoose={(field) => setAssigned(withAssignment(assigned, index, field))}
				/>
			))}
			<Button
				type="submit"
				disabled={save.fetching || stored.state !== 'ready'}
				loading={save.fetching}
			>
				{__('Save mapping', 'alphone-importer')}
			</Button>
			<Button
				variant="solid"
				disabled={commit.fetching || stored.state !== 'ready'}
				loading={commit.fetching}
				onClick={() => {
					void commitImport()
				}}
			>
				{__('Commit', 'alphone-importer')}
			</Button>
			{failure === undefined ? null : <ErrorNotice>{failure}</ErrorNotice>}
		</form>
	)
}

/**
 * Returns the toast a finished import raises.
 * @param counts - How many rows the import brought in, skipped and failed.
 * @returns The message naming the three counts.
 */
function finishedMessage(counts: { imported: number; skipped: number; failed: number }): string {
	return sprintf(
		__('Import finished: %(imported)s imported, %(skipped)s skipped, %(failed)s failed.', 'alphone-importer'),
		{
			imported: formatNumber(counts.imported),
			skipped: formatNumber(counts.skipped),
			failed: formatNumber(counts.failed),
		},
	)
}

/**
 * Returns the message a refused write shows, or nothing for a write that went through.
 * @param error - The failure the write met, absent when it went through.
 * @param fallback - The message for a failure that names no reason.
 * @returns The message, or undefined.
 */
function failureOf(error: GraphFailure | undefined, fallback: string): string | undefined {
	return error === undefined ? undefined : validationMessage(graphError(error), fallback)
}

/**
 * Renders the field chooser for one column of the import.
 * @returns The column select.
 */
function ColumnSelect({
	column,
	index,
	fields,
	chosen,
	onChoose,
}: {
	column: string
	index: number
	fields: readonly ImportField[]
	chosen: string
	onChoose: (field: string) => void
}) {
	const labelOf = fieldLabeller(fields)
	const items = [
		{ value: unmapped, label: __('Not imported', 'alphone-importer') },
		...fields.map((field) => ({ value: field.name, label: labelOf(field.name) })),
	]
	return (
		<SelectControl
			label={column === '' ? columnLabel(index) : column}
			items={items}
			value={chosenItem(items, chosen)}
			onValueChange={(item) => onChoose(chosenValue(item))}
		/>
	)
}

/**
 * Returns the item a chosen field stands for, or the unmapped item.
 * @param items - The items the select offers.
 * @param chosen - The field the column carries.
 * @returns The matching item.
 */
export function chosenItem(
	items: { value: string; label: string }[],
	chosen: string,
): { value: string; label: string } {
	return items.find((item) => item.value === chosen) ?? items[0]
}

/**
 * Returns the field a selection stands for, treating a cleared one as unmapped.
 * @param item - The chosen item, or null when the selection is cleared.
 * @returns The field name.
 */
export function chosenValue(item: { value: string | null } | null): string {
	return item?.value ?? unmapped
}

/**
 * Returns the assignments with one column reassigned.
 * @param assigned - The assignments so far.
 * @param index - The column being assigned.
 * @param field - The chosen field, or the unmapped value.
 * @returns The updated assignments.
 */
export function withAssignment(
	assigned: Record<string, string>,
	index: number,
	field: string,
): Record<string, string> {
	const next = { ...assigned }
	if (field === unmapped) {
		delete next[String(index)]
		return next
	}
	next[String(index)] = field
	return next
}

/**
 * Returns the column to field map the stored assignments stand for.
 * @param mapping - The assignments the import carries.
 * @returns The map the selects read.
 */
export function assignedOf(
	mapping: readonly { column: number; field: string }[],
): Record<string, string> {
	return Object.fromEntries(mapping.map((one) => [String(one.column), one.field]))
}

/**
 * Returns the wire assignments the stored map stands for.
 * @param assigned - The column to field assignments.
 * @returns The assignments in request shape.
 */
export function assignmentsOf(
	assigned: Record<string, string>,
): { column: number; field: string }[] {
	return Object.entries(assigned).map(([column, field]) => ({
		column: Number(column),
		field,
	}))
}
