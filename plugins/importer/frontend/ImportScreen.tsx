// SPDX-License-Identifier: AGPL-3.0-or-later

import {
	Button,
	ErrorNotice,
	LoadingRows,
	PageScreen,
	SelectControl,
	Text,
	validationMessage,
} from '@alphone/frontend-sdk'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { Suspense, lazy, useState } from 'react'

import { commitImport, fetchFields, fetchImport, fetchRows, saveMapping } from './api'
import type { ImportDetail, ImportField } from './api'
import { importsQueryKey } from './ImportsScreen'

const RowsTable = lazy(() => import('./RowsTable'))

// unmapped is the select value a column carries until a field is chosen.
const unmapped = 'not-imported'

/**
 * Renders one import: its column mapping, its staged rows, and the commit.
 * @returns The import screen.
 */
export function ImportScreen({ importId }: { importId: string }) {
	const stored = useQuery({
		queryKey: [...importsQueryKey, importId],
		queryFn: () => fetchImport(importId),
	})
	const rows = useQuery({
		queryKey: [...importsQueryKey, importId, 'rows'],
		queryFn: () => fetchRows(importId),
	})

	if (stored.isPending || rows.isPending) {
		return <Text role="status">Loading import…</Text>
	}
	if (stored.isError || rows.isError) {
		return <ErrorNotice>The import could not be loaded.</ErrorNotice>
	}
	return (
		<PageScreen title={stored.data.filename}>
			<MappingForm stored={stored.data} />
			<Text variant="heading-sm" render={<h2 />}>
				Rows
			</Text>
			<Suspense fallback={<LoadingRows label="Loading the preview…" rows={3} />}>
				<RowsTable stored={stored.data} rows={rows.data} />
			</Suspense>
		</PageScreen>
	)
}

/**
 * Renders the column assignments and the control that commits them.
 * @returns The mapping form.
 */
function MappingForm({ stored }: { stored: ImportDetail }) {
	const queryClient = useQueryClient()
	const fields = useQuery({ queryKey: ['importer', 'fields'], queryFn: fetchFields })
	const [assigned, setAssigned] = useState<Record<string, string>>(stored.mapping)
	const save = useMutation({
		mutationFn: () => saveMapping(stored.id, assignmentsOf(assigned)),
		onSuccess: () => queryClient.invalidateQueries({ queryKey: importsQueryKey }),
	})
	const commit = useMutation({
		mutationFn: () => commitImport(stored.id),
		onSuccess: () => queryClient.invalidateQueries({ queryKey: importsQueryKey }),
	})

	if (fields.isPending) {
		return <Text role="status">Loading fields…</Text>
	}
	if (fields.isError) {
		return <ErrorNotice>The fields could not be loaded.</ErrorNotice>
	}
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
					fields={fields.data}
					chosen={assigned[String(index)] ?? unmapped}
					onChoose={(field) => setAssigned(withAssignment(assigned, index, field))}
				/>
			))}
			<Button type="submit" disabled={save.isPending || stored.state !== 'ready'}>
				Save mapping
			</Button>
			<Button
				variant="solid"
				disabled={commit.isPending || stored.state !== 'ready'}
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
	fields: ImportField[]
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
