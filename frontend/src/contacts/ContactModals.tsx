// SPDX-License-Identifier: AGPL-3.0-or-later

import {
	RenameBody,
	__,
	graphError,
	markLatest,
	useGraph,
	useToaster,
	validationMessage,
} from '@alphone/frontend-sdk'
import type { ListRun, UnsavedText } from '@alphone/frontend-sdk'
import type { RenderModalProps } from '@alphone/frontend-sdk/dataviews'

import type { ContactRow } from './contactFields'
import { renameContactMutation } from './operations'

/**
 * Renders the modal writing a new name for one contact, closed the moment Save is pressed, its failure in a toast.
 * @param props - The contact, the handler closing the modal, the names left unsaved, the list run and its reload.
 * @returns The rename form.
 */
export function RenameModal({
	items: [contact],
	closeModal,
	unsaved,
	run,
	reload,
}: RenderModalProps<ContactRow> & { unsaved: UnsavedText; run: ListRun; reload: () => Promise<unknown> }) {
	const graph = useGraph()
	const toaster = useToaster()
	const save = (name: string) => {
		closeModal?.()
		unsaved.delete(contact.id)
		const stillLatest = markLatest(unsaved, contact.id)
		void graph.client
			.mutation(renameContactMutation, { id: contact.id, name })
			.toPromise()
			.then((result) => {
				if (result.error) {
					if (stillLatest()) {
						unsaved.set(contact.id, name)
					}
					toaster.show(validationMessage(graphError(result.error), __('The name could not be updated.', 'alphone')))
					return
				}
				toaster.show(__('Name updated.', 'alphone'))
				const ended = run.onRefresh()
				void reload().then(ended, ended)
			})
	}

	return (
		<RenameBody
			name={unsaved.get(contact.id) ?? contact.name}
			fieldLabel={__('Name', 'alphone')}
			submitLabel={__('Save', 'alphone')}
			cancelLabel={__('Cancel', 'alphone')}
			onSubmit={save}
			onCancel={closeModal}
		/>
	)
}
