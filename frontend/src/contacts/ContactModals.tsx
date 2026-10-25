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
import type { UnsavedText } from '@alphone/frontend-sdk'
import type { RenderModalProps } from '@alphone/frontend-sdk/dataviews'

import type { ContactRow } from './contactFields'
import { renameContactMutation } from './operations'

/** contactPageOperation names the query the contacts list reads, rerun once a contact changed. */
const contactPageOperation = 'ContactPage'

/**
 * Renders the modal writing a new name for one contact, closed the moment Save is pressed, its failure in a toast.
 * @param props - The contact, the handler closing the modal, and the names failed renames left unsaved.
 * @returns The rename form.
 */
export function RenameModal({
	items: [contact],
	closeModal,
	unsaved,
}: RenderModalProps<ContactRow> & { unsaved: UnsavedText }) {
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
				graph.refetch([contactPageOperation])
				toaster.show(__('Name updated.', 'alphone'))
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
