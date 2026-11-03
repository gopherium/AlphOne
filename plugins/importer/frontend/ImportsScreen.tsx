// SPDX-License-Identifier: AGPL-3.0-or-later

import {
	ErrorNotice,
	FileButton,
	ListEmpty,
	ListNoResults,
	PageScreen,
	__,
	_x,
	openOnTap,
	useAdminSettings,
	useGraph,
	useGraphMutation,
	useGraphQuery,
	useListRun,
	useListView,
	upload as uploadIcon,
	useToaster,
	writeFailure,
} from '@alphone/frontend-sdk'
import type { ListRun } from '@alphone/frontend-sdk'
import { DataViews, filterSortAndPaginate } from '@alphone/frontend-sdk/dataviews'
import { useNavigate } from '@tanstack/react-router'
import { useMemo } from 'react'

import { importFields } from './importFields'
import type { ImportSummary } from './importFields'
import { importUploadMutation, importsQuery } from './operations'

/** noImports stands in for the list until the imports arrive. */
const noImports: ImportSummary[] = []

/**
 * Renders what the list shows when no import is on it.
 * @param props - Whether imports exist that the view narrowed away.
 * @returns The empty state.
 */
function ImportsEmpty({ narrowed }: { narrowed: boolean }) {
	if (narrowed) {
		return <ListNoResults />
	}
	return (
		<ListEmpty
			icon={uploadIcon}
			title={__('No imports yet', 'alphone-importer')}
			description={__('Upload a CSV or Excel file to start one.', 'alphone-importer')}
		/>
	)
}

/**
 * Starts an upload, raising a toast that opens the stored file's mapping or says what failed, then reloading the list.
 * @param run - What the list does around its own reload.
 * @returns The upload state beside the control starting one.
 */
function useUpload(run: ListRun) {
	const graph = useGraph()
	const toaster = useToaster()
	const navigate = useNavigate()
	const [upload, startUpload] = useGraphMutation(importUploadMutation)
	const start = async (file: File) => {
		const result = await startUpload({ file })
		if (!result.data) {
			toaster.show(writeFailure(__('Could not import the file.', 'alphone-importer'), result.error))
			graph.refetch(['Imports'])
			return
		}
		const importId = result.data.importUpload.id
		toaster.show(__('File uploaded.', 'alphone-importer'), {
			label: __('Open', 'alphone-importer'),
			onAct: () => void navigate({ to: '/import/$importId', params: { importId } }),
		})
		const ended = run.onRefresh()
		await graph.client.query(importsQuery, {}, { requestPolicy: 'network-only' }).toPromise().then(ended, ended)
	}
	return { upload, start }
}

/**
 * Renders the imports as a list to search, filter, sort and page, beside the button starting a new one.
 * @returns The imports screen.
 */
export function ImportsScreen() {
	const [imports] = useGraphQuery({ query: importsQuery, requestPolicy: 'cache-and-network' })
	const navigate = useNavigate()
	const { settings, failed } = useAdminSettings()
	const list = useListView({
		fields: ['state', 'rowCount', 'importedCount', 'skippedCount', 'failedCount', 'started'],
		phoneFields: ['state', 'started'],
		titleField: 'filename',
		sort: { field: 'started', direction: 'desc' },
		perPage: settings?.listPageSize,
	})
	const { run, refreshing, region } = useListRun('imports', list.onChangeSelection)
	const { upload, start } = useUpload(run)
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
			headingLevel={2}
			subtitle={__('Bring contacts in from CSV and Excel files.', 'alphone-importer')}
			actions={
				<FileButton
					accept=".csv,.xlsx"
					icon={uploadIcon}
					busy={upload.fetching}
					inputLabel={__('Contacts file', 'alphone-importer')}
					onChoose={([file]) => void start(file)}
				>
					{__('Upload', 'alphone-importer')}
				</FileButton>
			}
			list
			listLabel={__('Imports', 'alphone-importer')}
			listRef={region}
		>
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
					isLoading={imports.data === undefined || sizing || refreshing}
					searchLabel={__('Search imports…', 'alphone-importer')}
					config={settings === undefined ? undefined : { perPageSizes: settings.listPageSizes }}
					empty={<ImportsEmpty narrowed={stored.length > 0} />}
				/>
			)}
		</PageScreen>
	)
}
