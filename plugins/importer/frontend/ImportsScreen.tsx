// SPDX-License-Identifier: AGPL-3.0-or-later

import {
	EmptyState,
	ErrorNotice,
	LoadingRows,
	PageScreen,
	useGraph,
	useGraphQuery,
	validationMessage,
} from '@alphone/frontend-sdk'
import { useMutation } from '@tanstack/react-query'
import { Link } from '@tanstack/react-router'

import { uploadImport } from './api'
import { importerIcon } from './icon'
import { importsQuery } from './operations'

/** ImportRow is one stored import as the history document selects it. */
interface ImportRow {
	id: string
	filename: string
	state: string
	rowCount: number
	importedCount: number
	skippedCount: number
	failedCount: number
	createdAt: string
}

/**
 * Renders the import history beside the control that starts a new import.
 * @returns The imports screen.
 */
export function ImportsScreen() {
	const graph = useGraph()
	const [imports] = useGraphQuery({ query: importsQuery })
	const upload = useMutation({
		mutationFn: (file: File) => uploadImport(file),
		onSuccess: () => graph.refetch(['Imports']),
	})

	return (
		<PageScreen title="Import">
			<input
				type="file"
				accept=".csv,.xlsx"
				aria-label="Contacts file"
				disabled={upload.isPending}
				onChange={(event) => uploadChosen(event.target.files, upload.mutate)}
			/>
			{upload.isError ? (
				<ErrorNotice>
					{validationMessage(upload.error, 'The file could not be imported.')}
				</ErrorNotice>
			) : null}
			<ImportRows error={imports.error !== undefined} rows={imports.data?.imports} />
		</PageScreen>
	)
}

/**
 * Renders the loading, error, empty, and loaded states of the history.
 * @returns The history body.
 */
function ImportRows({
	error,
	rows,
}: {
	error: boolean
	rows: readonly ImportRow[] | undefined
}) {
	if (error) {
		return <ErrorNotice>Imports could not be loaded.</ErrorNotice>
	}
	if (!rows) {
		return <LoadingRows label="Loading imports…" />
	}
	if (rows.length === 0) {
		return (
			<EmptyState.Root className="godmin-empty">
				<EmptyState.Icon icon={importerIcon} />
				<EmptyState.Title>No imports yet.</EmptyState.Title>
				<EmptyState.Description>Choose a CSV or Excel file to start one.</EmptyState.Description>
			</EmptyState.Root>
		)
	}
	return (
		<div className="godmin-table-scroll" role="region" aria-label="Imports" tabIndex={0}>
			<table className="godmin-table">
				<thead>
					<tr>
						<th scope="col">File</th>
						<th scope="col">State</th>
						<th scope="col">Rows</th>
						<th scope="col">Imported</th>
						<th scope="col">Skipped</th>
						<th scope="col">Failed</th>
						<th scope="col">Started</th>
					</tr>
				</thead>
				<tbody>
					{rows.map((stored) => (
						<tr key={stored.id}>
							<td>
								<Link to="/import/$importId" params={{ importId: stored.id }}>
									{stored.filename}
								</Link>
							</td>
							<td>{stored.state}</td>
							<td>{stored.rowCount}</td>
							<td>{stored.importedCount}</td>
							<td>{stored.skippedCount}</td>
							<td>{stored.failedCount}</td>
							<td>{formatStarted(new Date(stored.createdAt))}</td>
						</tr>
					))}
				</tbody>
			</table>
		</div>
	)
}

/**
 * Hands the chosen file to upload, ignoring a cancelled dialog.
 * @param files - The files the input carries.
 * @param upload - The upload to start.
 */
export function uploadChosen(files: FileList | null, upload: (file: File) => void) {
	const file = files?.[0]
	if (file) {
		upload(file)
	}
}

/**
 * Formats the day an import was started.
 * @param started - The moment the import was stored.
 * @returns The formatted day.
 */
export function formatStarted(started: Date): string {
	return started.toLocaleDateString('en-GB', {
		day: 'numeric',
		month: 'short',
		year: 'numeric',
		timeZone: 'UTC',
	})
}
