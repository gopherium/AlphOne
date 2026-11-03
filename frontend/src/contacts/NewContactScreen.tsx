// SPDX-License-Identifier: AGPL-3.0-or-later

import {
	Button,
	PageScreen,
	ValidatedInputControl,
	__,
	useGraphMutation,
	useRefusedField,
	useToaster,
	writeFailure,
} from '@alphone/frontend-sdk'
import { useRef, useState } from 'react'

import { ContactBreadcrumbs } from './ContactScreen'
import { createContactMutation } from './operations'

/** CreatedContact is the contact the creation form hands back. */
export interface CreatedContact {
	id: string
	name: string
}

/**
 * Renders the new-contact form.
 * @returns The creation screen.
 */
export function NewContactScreen({
	onCreated,
}: {
	onCreated: (created: CreatedContact) => void
}) {
	const toaster = useToaster()
	const [name, setName] = useState('')
	const [create, runCreate] = useGraphMutation(createContactMutation)
	const form = useRef<HTMLFormElement>(null)
	const refusedName = useRefusedField(form, name)
	const submit = async () => {
		const result = await runCreate({ name })
		if (result.data) {
			toaster.show(__('Contact added.', 'alphone'))
			onCreated(result.data.createContact)
		} else if (!refusedName.refuse(result.error, name)) {
			toaster.show(writeFailure(__('Could not create the contact.', 'alphone'), result.error))
		}
	}

	return (
		<PageScreen title={__('New contact', 'alphone')} headingLevel={2} breadcrumbs={<ContactBreadcrumbs />}>
			<form
				ref={form}
				className="godmin-form"
				onSubmit={(event) => {
					event.preventDefault()
					void submit()
				}}
			>
				<div className="godmin-form__row">
					<ValidatedInputControl
						label={__('Name', 'alphone')}
						value={name}
						customValidity={refusedName.validity}
						onChange={(event) => setName(event.target.value)}
					/>
					<Button
						type="submit"
						disabled={name.trim() === '' || create.fetching}
						loading={create.fetching}
					>
						{__('Create contact', 'alphone')}
					</Button>
				</div>
			</form>
		</PageScreen>
	)
}
