// SPDX-License-Identifier: AGPL-3.0-or-later

import {
	Button,
	ErrorNotice,
	InputControl,
	PageScreen,
	__,
	graphError,
	useGraphMutation,
	validationMessage,
} from '@alphone/frontend-sdk'
import { useState } from 'react'

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
	const [name, setName] = useState('')
	const [create, runCreate] = useGraphMutation(createContactMutation)
	const submit = async () => {
		const result = await runCreate({ name })
		if (result.data) {
			onCreated(result.data.createContact)
		}
	}

	return (
		<PageScreen title={__('New contact', 'alphone')}>
			<form
				className="godmin-form"
				onSubmit={(event) => {
					event.preventDefault()
					void submit()
				}}
			>
				<InputControl
					label={__('Name', 'alphone')}
					value={name}
					onChange={(event) => setName(event.target.value)}
				/>
				<Button
					type="submit"
					disabled={name.trim() === '' || create.fetching}
					loading={create.fetching}
				>
					{__('Create contact', 'alphone')}
				</Button>
				{create.error ? (
					<ErrorNotice>
						{validationMessage(graphError(create.error), __('The contact could not be created.', 'alphone'))}
					</ErrorNotice>
				) : null}
			</form>
		</PageScreen>
	)
}
