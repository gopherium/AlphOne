// SPDX-License-Identifier: AGPL-3.0-or-later

import {
	Button,
	ErrorNotice,
	LoadingScreen,
	PageBreadcrumb,
	PageBreadcrumbs,
	PageScreen,
	SectionTitle,
	SelectControl,
	__,
	_x,
	formatNumber,
	sprintf,
	useGraph,
	useGraphMutation,
	useGraphQuery,
	useToaster,
	writeFailure,
} from '@alphone/frontend-sdk'
import type { GraphFailure } from '@alphone/frontend-sdk'
import { Link, useParams } from '@tanstack/react-router'
import { Suspense, lazy, useState } from 'react'

import { columnLabel } from './column'
import { fieldLabeller } from './fieldLabels'
import type { ImportDetailQuery } from './gql/graphql'
import {
	importCommitMutation,
	importDetailQuery,
	importSetMappingMutation,
} from './operations'
import { useImportRows } from './useImportRows'
import type { ImportRowsRead } from './useImportRows'

const RowsTable = lazy(() => import('./RowsTable'))

/** StoredImport is one import as the detail document selects it. */
export type StoredImport = NonNullable<ImportDetailQuery['importJob']>

/** ImportField is one target field as the detail document selects it. */
export type ImportField = ImportDetailQuery['importFields'][number]

// unmapped is the select value a column carries until a field is chosen.
const unmapped = 'not-imported'

/**
 * Renders the import named in the route path.
 * @returns The import screen.
 */
export function ImportRouteScreen() {
	const { importId } = useParams({ from: '/import/$importId' })
	return <ImportScreen importId={importId} />
}

/**
 * Renders the link back to the list of imports.
 * @returns The breadcrumbs of an import page.
 */
function ImportBreadcrumbs() {
	return (
		<PageBreadcrumbs label={__('Breadcrumbs', 'alphone-importer')}>
			<PageBreadcrumb render={<Link to="/import" activeOptions={{ exact: true }} />}>
				{_x('Import', 'admin section', 'alphone-importer')}
			</PageBreadcrumb>
		</PageBreadcrumbs>
	)
}

/**
 * Renders the page of an import that could not be loaded.
 * @returns The failed page.
 */
function FailedImport() {
	return (
		<PageScreen
			title={_x('Import', 'admin section', 'alphone-importer')}
			breadcrumbs={<ImportBreadcrumbs />}
			headingLevel={2}
		>
			<ErrorNotice>{__('The import could not be loaded.', 'alphone-importer')}</ErrorNotice>
		</PageScreen>
	)
}

/**
 * Renders one import: its column mapping, its staged rows, and the commit.
 * @returns The import screen.
 */
export function ImportScreen({ importId }: { importId: string }) {
	const [detail] = useGraphQuery({ query: importDetailQuery, variables: { id: importId } })
	const staged = useImportRows(importId)

	if (detail.error) {
		return <FailedImport />
	}
	if (!detail.data) {
		return (
			<PageScreen
				title={_x('Import', 'admin section', 'alphone-importer')}
				breadcrumbs={<ImportBreadcrumbs />}
				headingLevel={2}
			>
				<LoadingScreen label={__('Loading import…', 'alphone-importer')} />
			</PageScreen>
		)
	}
	const { importJob: stored, importFields } = detail.data
	if (!stored) {
		return <FailedImport />
	}
	return (
		<PageScreen title={stored.filename} breadcrumbs={<ImportBreadcrumbs />} headingLevel={2}>
			<MappingForm stored={stored} fields={importFields} />
			<SectionTitle>{__('Rows', 'alphone-importer')}</SectionTitle>
			<StagedRows stored={stored} staged={staged} fields={importFields} />
		</PageScreen>
	)
}

/**
 * Renders the staged rows once every page of them is read, or why they are not shown.
 * @returns The rows table, its loading state, or the failure.
 */
function StagedRows({
	stored,
	staged,
	fields,
}: {
	stored: StoredImport
	staged: ImportRowsRead
	fields: readonly ImportField[]
}) {
	const loading = <LoadingScreen label={__('Loading the preview…', 'alphone-importer')} />
	if (staged.failed) {
		return <ErrorNotice>{__('The import could not be loaded.', 'alphone-importer')}</ErrorNotice>
	}
	if (staged.rows === undefined) {
		return loading
	}
	return (
		<Suspense fallback={loading}>
			<RowsTable stored={stored} rows={staged.rows} fields={fields} />
		</Suspense>
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
	const refresh = () => graph.refetch(['ImportDetail', 'Imports'])
	const failed = (sentence: string, error: GraphFailure | undefined) => {
		toaster.show(writeFailure(sentence, error))
		if (!error?.networkError) {
			refresh()
		}
	}
	const saveMapping = async () => {
		const result = await startSave({ id: stored.id, assignments: assignmentsOf(assigned) })
		if (!result.data) {
			failed(__('Could not save the mapping.', 'alphone-importer'), result.error)
			return
		}
		toaster.show(__('Mapping saved.', 'alphone-importer'))
		refresh()
	}
	const commitImport = async () => {
		const result = await startCommit({ id: stored.id })
		if (!result.data) {
			failed(__('Could not commit the import.', 'alphone-importer'), result.error)
			return
		}
		toaster.show(finishedMessage(result.data.importCommit))
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
