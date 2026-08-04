// SPDX-License-Identifier: AGPL-3.0-or-later

import {
	EmptyState,
	ErrorNotice,
	PageScreen,
	Text,
	validationMessage,
} from '@alphone/frontend-sdk'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { Link } from '@tanstack/react-router'

import { fetchImports, uploadImport } from './api'
import type { ImportSummary } from './api'
import { importerIcon } from './icon'

export const importsQueryKey = ['importer', 'imports']

/**
 * Renders the import history beside the control that starts a new import.
 * @returns The imports screen.
 */
export function ImportsScreen() {
	const queryClient = useQueryClient()
	const imports = useQuery({ queryKey: importsQueryKey, queryFn: fetchImports })
	const upload = useMutation({
		mutationFn: (file: File) => uploadImport(file),
		onSuccess: () => queryClient.invalidateQueries({ queryKey: importsQueryKey }),
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
			<ImportRows imports={imports} />
		</PageScreen>
	)
}

/**
 * Renders the loading, error, empty, and loaded states of the history.
 * @returns The history body.
 */
function ImportRows({
	imports,
}: {
	imports: ReturnType<typeof useQuery<ImportSummary[], Error>>
}) {
	if (imports.isPending) {
		return <Text role="status">Loading imports…</Text>
	}
	if (imports.isError) {
		return <ErrorNotice>Imports could not be loaded.</ErrorNotice>
	}
	if (imports.data.length === 0) {
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
					{imports.data.map((stored) => (
						<tr key={stored.id}>
							<td>
								<Link to="/import/$importId" params={{ importId: stored.id }}>
									{stored.filename}
								</Link>
							</td>
							<td>{stored.state}</td>
							<td>{stored.row_count}</td>
							<td>{stored.imported_count}</td>
							<td>{stored.skipped_count}</td>
							<td>{stored.failed_count}</td>
							<td>{formatStarted(stored.created_at)}</td>
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
