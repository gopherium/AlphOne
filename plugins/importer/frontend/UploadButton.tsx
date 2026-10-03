// SPDX-License-Identifier: AGPL-3.0-or-later

import { Button, __ } from '@alphone/frontend-sdk'
import { useRef } from 'react'

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
 * Renders the compact Upload button of the page header, opening the file dialog for a CSV or Excel file.
 * @param props - Whether an upload is running, and the upload to start with the chosen file.
 * @returns The button beside its hidden file input.
 */
export function UploadButton({ busy, onChoose }: { busy: boolean; onChoose: (file: File) => void }) {
	const input = useRef<HTMLInputElement>(null)
	return (
		<>
			<Button
				variant="solid"
				size="compact"
				disabled={busy}
				loading={busy}
				onClick={() => (input.current as HTMLInputElement).click()}
			>
				{__('Upload', 'alphone-importer')}
			</Button>
			<input
				ref={input}
				type="file"
				accept=".csv,.xlsx"
				hidden
				aria-label={__('Contacts file', 'alphone-importer')}
				onChange={(event) => {
					uploadChosen(event.target.files, onChoose)
					event.target.value = ''
				}}
			/>
		</>
	)
}
