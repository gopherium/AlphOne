// SPDX-License-Identifier: AGPL-3.0-or-later

import {
	Button,
	ErrorNotice,
	LoadingRows,
	LoadingScreen,
	PageScreen,
	SelectControl,
	Text,
	useGraph,
	useGraphQuery,
	validationMessage,
} from '@alphone/frontend-sdk'
import { useMutation } from '@tanstack/react-query'
import { Suspense, lazy, useState } from 'react'

import { commitImport, saveMapping } from './api'
import type { ImportDetailQuery } from './gql/graphql'
import { importDetailQuery } from './operations'

const RowsTable = lazy(() => import('./RowsTable'))

/** StoredImport is one import as the detail document selects it. */
export type StoredImport = NonNullable<ImportDetailQuery['importJob']>

/** ImportField is one target field as the detail document selects it. */
type ImportField = ImportDetailQuery['importFields'][number]

// unmapped is the select value a column carries until a field is chosen.
const unmapped = 'not-imported'

/**
 * Renders one import: its column mapping, its staged rows, and the commit.
 * @returns The import screen.
 */
export function ImportScreen({ importId }: { importId: string }) {
	const [detail] = useGraphQuery({ query: importDetailQuery, variables: { id: importId } })

	if (detail.error) {
		return <ErrorNotice>The import could not be loaded.</ErrorNotice>
	}
	if (!detail.data) {
		return (
			<PageScreen title="Import">
				<LoadingScreen label="Loading import…" />
			</PageScreen>
		)
	}
	const { importJob: stored, importFields } = detail.data
	if (!stored) {
		return <ErrorNotice>The import could not be loaded.</ErrorNotice>
	}
	return (
		<PageScreen title={stored.filename}>
			<MappingForm stored={stored} fields={importFields} />
			<Text variant="heading-sm" render={<h2 />}>
				Rows
			</Text>
			<Suspense fallback={<LoadingRows label="Loading the preview…" rows={3} />}>
				<RowsTable stored={stored} rows={stored.rows} />
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
	const [assigned, setAssigned] = useState<Record<string, string>>(assignedOf(stored.mapping))
	const save = useMutation({
		mutationFn: () => saveMapping(stored.id, assignmentsOf(assigned)),
		onSuccess: () => graph.refetch(['ImportDetail', 'Imports']),
	})
	const commit = useMutation({
		mutationFn: () => commitImport(stored.id),
		onSuccess: () => graph.refetch(['ImportDetail', 'Imports']),
	})

	return (
		<form
			className="godmin-form"
			onSubmit={(event) => {
				event.preventDefault()
				save.mutate()
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
				disabled={save.isPending || stored.state !== 'ready'}
				loading={save.isPending}
			>
				Save mapping
			</Button>
			<Button
				variant="solid"
				disabled={commit.isPending || stored.state !== 'ready'}
				loading={commit.isPending}
				onClick={() => commit.mutate()}
			>
				Commit
			</Button>
			<MappingNotice save={save} commit={commit} />
		</form>
	)
}

/**
 * Renders whichever mapping failure the caller needs to see.
 * @returns The failure notice.
 */
function MappingNotice({
	save,
	commit,
}: {
	save: { isError: boolean; error: unknown }
	commit: { isError: boolean; error: unknown }
}) {
	if (save.isError) {
		return <ErrorNotice>{validationMessage(save.error, 'The mapping could not be saved.')}</ErrorNotice>
	}
	if (commit.isError) {
		return <ErrorNotice>{validationMessage(commit.error, 'The import could not be committed.')}</ErrorNotice>
	}
	return null
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
	const items = [
		{ value: unmapped, label: 'Not imported' },
		...fields.map((field) => ({ value: field.name, label: field.label })),
	]
	return (
		<SelectControl
			label={column === '' ? `Column ${index + 1}` : column}
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
