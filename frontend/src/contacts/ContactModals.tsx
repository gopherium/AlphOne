// SPDX-License-Identifier: AGPL-3.0-or-later

import {
	Button,
	ErrorNotice,
	InputControl,
	Stack,
	__,
	graphError,
	useGraph,
	useGraphMutation,
	useToaster,
	validationMessage,
} from '@alphone/frontend-sdk'
import type { RenderModalProps } from '@alphone/frontend-sdk/dataviews'
import { useState } from 'react'

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
	const [name, setName] = useState(contact.name)
	const [renamed, rename] = useGraphMutation(renameContactMutation)
	const submit = async () => {
		const result = await rename({ id: contact.id, name })
		if (result.error) {
			return
		}
		graph.refetch([contactPageOperation])
		toaster.show(__('Contact renamed.', 'alphone'))
		closeModal?.()
	}

	return (
		<form
			onSubmit={(event) => {
				event.preventDefault()
				void submit()
			}}
		>
			<Stack direction="column" gap="lg">
				<InputControl label={__('Name', 'alphone')} value={name} onChange={(event) => setName(event.target.value)} />
				{renamed.error ? (
					<ErrorNotice>
						{validationMessage(graphError(renamed.error), __('The contact could not be renamed.', 'alphone'))}
					</ErrorNotice>
				) : null}
				<Stack direction="row" gap="sm" justify="flex-end">
					<Button variant="minimal" onClick={closeModal}>
						{__('Cancel', 'alphone')}
					</Button>
					<Button
						type="submit"
						loading={renamed.fetching}
						disabled={name.trim() === '' || name === contact.name}
					>
						{__('Rename', 'alphone')}
					</Button>
				</Stack>
			</Stack>
		</form>
	)
}
