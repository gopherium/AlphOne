// SPDX-License-Identifier: AGPL-3.0-or-later

import { RenameDialog, __, dialogWrite, graphError, useGraph, validationMessage } from '@alphone/frontend-sdk'
import type { BulkWords, DialogRun } from '@alphone/frontend-sdk'

import type { ContactRow } from './contactFields'
import { renameContactMutation } from './operations'

/** RenameProps are what the contact rename reads. */
interface RenameProps {
	/** contact is the contact the reader asked to rename. */
	contact: ContactRow
	/** write renames it through the list and answers the dialog how the write went. */
	write: DialogRun<ContactRow>
	/** onClose takes the dialog away once its card has left. */
	onClose: () => void
}

/**
 * Returns the words a contact rename reports in, read fresh so the loaded catalogue answers.
 * @returns The words for a rename that worked and for one that failed or ran past the time limit.
 */
function renameWords(): BulkWords<ContactRow> {
	return {
		done: () => __('Name updated.', 'alphone'),
		failed: ([{ error }]) => validationMessage(graphError(error), __('The name could not be updated.', 'alphone')),
	}
}

/**
 * Renders the dialog writing a new name for one contact, open while the write runs and its failure shown inside.
 * @param props - The contact, the list write and the handler taking the dialog away.
 * @returns The rename dialog.
 */
export function RenameContactDialog({ contact, write, onClose }: RenameProps) {
	const graph = useGraph()
	return (
		<RenameDialog
			title={__('Rename', 'alphone')}
			name={contact.name}
			fieldLabel={__('Name', 'alphone')}
			submitLabel={__('Save', 'alphone')}
			cancelLabel={__('Cancel', 'alphone')}
			onSubmit={(name, signal) =>
				write(
					[contact],
					(row) => dialogWrite(graph.client, renameContactMutation, { id: row.id, name }, signal),
					renameWords(),
					signal,
				)
			}
			onClose={onClose}
		/>
	)
}
