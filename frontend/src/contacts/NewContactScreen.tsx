// SPDX-License-Identifier: AGPL-3.0-or-later

import { Button, InputControl, Text } from '@alphone/frontend-sdk'
import { useMutation, useQueryClient } from '@tanstack/react-query'
import { useState } from 'react'

import { ValidationError, createContact } from './api'
import type { Contact } from './api'

/**
 * Copy for a failed contact creation.
 * @param error - The mutation error.
 * @returns The message shown under the form.
 */
function createErrorText(error: unknown): string {
	if (error instanceof ValidationError) {
		return error.message
	}
	return 'The contact could not be created.'
}

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
		<div className="alphone-contacts">
			<h1>New contact</h1>
			<form
				className="alphone-contacts__form"
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
					<Text role="alert">{createErrorText(create.error)}</Text>
				) : null}
			</form>
		</div>
	)
}
