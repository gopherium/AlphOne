// SPDX-License-Identifier: AGPL-3.0-or-later

import {
	Button,
	ErrorNotice,
	InputControl,
	PageScreen,
	validationMessage,
} from '@alphone/frontend-sdk'
import { useMutation, useQueryClient } from '@tanstack/react-query'
import { useState } from 'react'

import { createContact } from './api'
import type { Contact } from './api'

/**
 * Renders the new-contact form.
 * @returns The creation screen.
 */
export function NewContactScreen({ onCreated }: { onCreated: (created: Contact) => void }) {
	const queryClient = useQueryClient()
	const [name, setName] = useState('')
	const create = useMutation({
		mutationFn: () => createContact(name),
		onSuccess: (created) => {
			void queryClient.invalidateQueries({ queryKey: ['contacts'] })
			onCreated(created)
		},
	})

	return (
		<PageScreen title="New contact">
			<form
				className="alphone-form"
				onSubmit={(event) => {
					event.preventDefault()
					create.mutate()
				}}
			>
				<InputControl
					label="Name"
					value={name}
					onChange={(event) => setName(event.target.value)}
				/>
				<Button type="submit" disabled={name.trim() === '' || create.isPending}>
					Create contact
				</Button>
				{create.isError ? (
					<ErrorNotice>
						{validationMessage(create.error, 'The contact could not be created.')}
					</ErrorNotice>
				) : null}
			</form>
		</PageScreen>
	)
}
