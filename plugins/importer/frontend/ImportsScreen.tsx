// SPDX-License-Identifier: AGPL-3.0-or-later

import {
	EmptyState,
	ErrorNotice,
	LoadingRows,
	PageScreen,
	__,
	formatDate,
	_x,
	graphError,
	useGraph,
	useGraphMutation,
	useGraphQuery,
	validationMessage,
} from '@alphone/frontend-sdk'
import { Link } from '@tanstack/react-router'

import { importerIcon } from './icon'
import { importUploadMutation, importsQuery } from './operations'

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
	const [upload, startUpload] = useGraphMutation(importUploadMutation)

	return (
		<PageScreen title={__('Import', 'alphone-importer')}>
			<input
				type="file"
				accept=".csv,.xlsx"
				aria-label={__('Contacts file', 'alphone-importer')}
				disabled={upload.fetching}
				onChange={(event) =>
					uploadChosen(event.target.files, (file) => {
						void startUpload({ file }).then(() => graph.refetch(['Imports']))
					})
				}
			/>
			{upload.error ? (
				<ErrorNotice>
					{validationMessage(graphError(upload.error), __('The file could not be imported.', 'alphone-importer'))}
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
		return <ErrorNotice>{__('Imports could not be loaded.', 'alphone-importer')}</ErrorNotice>
	}
	if (!rows) {
		return <LoadingRows label={__('Loading imports…', 'alphone-importer')} />
	}
	if (rows.length === 0) {
		return (
			<EmptyState.Root className="godmin-empty">
				<EmptyState.Icon icon={importerIcon} />
				<EmptyState.Title>{__('No imports yet.', 'alphone-importer')}</EmptyState.Title>
				<EmptyState.Description>
					{__('Choose a CSV or Excel file to start one.', 'alphone-importer')}
				</EmptyState.Description>
			</EmptyState.Root>
		)
	}
	return (
		<div className="godmin-table-scroll" role="region" aria-label={__('Imports', 'alphone-importer')} tabIndex={0}>
			<table className="godmin-table">
				<thead>
					<tr>
						<th scope="col">{__('File', 'alphone-importer')}</th>
						<th scope="col">{__('State', 'alphone-importer')}</th>
						<th scope="col">{__('Rows', 'alphone-importer')}</th>
						<th scope="col">{_x('Imported', 'import count', 'alphone-importer')}</th>
						<th scope="col">{_x('Skipped', 'import count', 'alphone-importer')}</th>
						<th scope="col">{_x('Failed', 'import count', 'alphone-importer')}</th>
						<th scope="col">{__('Started', 'alphone-importer')}</th>
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
	return formatDate(started, {
		day: 'numeric',
		month: 'short',
		year: 'numeric',
		timeZone: 'UTC',
	})
}
