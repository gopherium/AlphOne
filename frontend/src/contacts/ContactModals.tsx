// SPDX-License-Identifier: AGPL-3.0-or-later

import {
	RenameBody,
	__,
	graphError,
	useGraph,
	useGraphMutation,
	useToaster,
	validationMessage,
} from '@alphone/frontend-sdk'
import type { RenderModalProps } from '@alphone/frontend-sdk/dataviews'

import type { ContactRow } from './contactFields'
import { renameContactMutation } from './operations'

/** contactPageOperation names the query the contacts list reads, rerun once a contact changed. */
const contactPageOperation = 'ContactPage'

/**
 * Renders the modal writing a new name for one contact.
 * @param props - The contact and the handler closing the modal.
 * @returns The rename form.
 */
export function RenameModal({ items: [contact], closeModal }: RenderModalProps<ContactRow>) {
	const graph = useGraph()
	const toaster = useToaster()
	const [renamed, rename] = useGraphMutation(renameContactMutation)
	const submit = async (name: string) => {
		const result = await rename({ id: contact.id, name })
		if (result.error) {
			return
		}
		graph.refetch([contactPageOperation])
		toaster.show(__('Name updated.', 'alphone'))
		closeModal?.()
	}

	return (
		<RenameBody
			name={contact.name}
			fieldLabel={__('Name', 'alphone')}
			submitLabel={__('Rename', 'alphone')}
			cancelLabel={__('Cancel', 'alphone')}
			busy={renamed.fetching}
			failure={
				renamed.error
					? validationMessage(graphError(renamed.error), __('The name could not be updated.', 'alphone'))
					: undefined
			}
			onSubmit={(name) => void submit(name)}
			onCancel={renamed.fetching ? undefined : closeModal}
		/>
	)
}
