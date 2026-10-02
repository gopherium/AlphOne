// SPDX-License-Identifier: AGPL-3.0-or-later

import {
	EmptyState,
	ErrorNotice,
	PageScreen,
	__,
	_x,
	graphError,
	openOnTap,
	useAdminSettings,
	useGraph,
	useGraphMutation,
	useGraphQuery,
	useListView,
	useToaster,
	validationMessage,
} from '@alphone/frontend-sdk'
import { DataViews, filterSortAndPaginate } from '@alphone/frontend-sdk/dataviews'
import { useNavigate } from '@tanstack/react-router'
import { useMemo } from 'react'

import { importerIcon } from './icon'
import { importFields } from './importFields'
import type { ImportSummary } from './importFields'
import { importUploadMutation, importsQuery } from './operations'
import { UploadButton } from './UploadButton'

/** noImports stands in for the list until the imports arrive. */
const noImports: ImportSummary[] = []

/**
 * Renders what the list shows when no import is on it.
 * @param props - Whether imports exist that the view narrowed away.
 * @returns The empty state.
 */
function ImportsEmpty({ narrowed }: { narrowed: boolean }) {
	return (
		<EmptyState.Root className="godmin-empty">
			<EmptyState.Icon icon={importerIcon} />
			{narrowed ? (
				<EmptyState.Title>{__('No imports found.', 'alphone-importer')}</EmptyState.Title>
			) : (
				<>
					<EmptyState.Title>{__('No imports yet.', 'alphone-importer')}</EmptyState.Title>
					<EmptyState.Description>
						{__('Upload a CSV or Excel file to start one.', 'alphone-importer')}
					</EmptyState.Description>
				</>
			)}
		</EmptyState.Root>
	)
}

/**
 * Starts an upload, confirming a stored file with a toast that opens its mapping.
 * @returns The upload state beside the control starting one.
 */
function useUpload() {
	const graph = useGraph()
	const toaster = useToaster()
	const navigate = useNavigate()
	const [upload, startUpload] = useGraphMutation(importUploadMutation)
	const start = async (file: File) => {
		const result = await startUpload({ file })
		if (result.data) {
			const importId = result.data.importUpload.id
			toaster.show(__('File uploaded.', 'alphone-importer'), {
				label: __('Open', 'alphone-importer'),
				onAct: () => void navigate({ to: '/import/$importId', params: { importId } }),
			})
		}
		graph.refetch(['Imports'])
	}
	return { upload, start }
}

/**
 * Renders the imports as a list to search, filter, sort and page, beside the button starting a new one.
 * @returns The imports screen.
 */
export function ImportsScreen() {
	const [imports] = useGraphQuery({ query: importsQuery, requestPolicy: 'cache-and-network' })
	const { upload, start } = useUpload()
	const navigate = useNavigate()
	const { settings, failed } = useAdminSettings()
	const list = useListView({
		fields: ['state', 'rowCount', 'importedCount', 'skippedCount', 'failedCount', 'started'],
		phoneFields: ['state', 'started'],
		titleField: 'filename',
		sort: { field: 'started', direction: 'desc' },
		perPage: settings?.listPageSize,
	})
	const onChangeSelection = openOnTap(list, (importId) => {
		void navigate({ to: '/import/$importId', params: { importId } })
	})
	const stored = imports.data?.imports ?? noImports
	const fields = useMemo(() => importFields(), [])
	const shown = filterSortAndPaginate(stored, list.view, fields)
	const sizing = list.view.perPage === undefined && !failed

	return (
		<PageScreen
			title={_x('Import', 'admin section', 'alphone-importer')}
			subtitle={__('Bring contacts in from CSV and Excel files.', 'alphone-importer')}
			actions={<UploadButton busy={upload.fetching} onChoose={(file) => void start(file)} />}
			list
		>
			{upload.error ? (
				<ErrorNotice>
					{validationMessage(graphError(upload.error), __('The file could not be imported.', 'alphone-importer'))}
				</ErrorNotice>
			) : null}
			{imports.error ? (
				<ErrorNotice>{__('Imports could not be loaded.', 'alphone-importer')}</ErrorNotice>
			) : (
				<DataViews<ImportSummary>
					data={shown.data}
					paginationInfo={shown.paginationInfo}
					fields={fields}
					view={list.view}
					onChangeView={list.onChangeView}
					defaultLayouts={list.defaultLayouts}
					selection={list.selection}
					onChangeSelection={onChangeSelection}
					isLoading={imports.data === undefined || sizing}
					searchLabel={__('Search imports…', 'alphone-importer')}
					config={settings === undefined ? undefined : { perPageSizes: settings.listPageSizes }}
					empty={<ImportsEmpty narrowed={stored.length > 0} />}
				/>
			)}
		</PageScreen>
	)
}
