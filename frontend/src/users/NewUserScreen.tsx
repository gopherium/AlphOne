// SPDX-License-Identifier: AGPL-3.0-or-later

import { Button, ErrorNotice, InputControl, PageScreen, __ } from '@alphone/frontend-sdk'
import {
	EmailTakenError,
	ValidationError,
	createUser,
	usersQueryKey,
} from '@gopherium/react-auth/admin'
import { useMutation, useQueryClient } from '@tanstack/react-query'
import { useState } from 'react'

/**
 * Returns the message shown under the form for a failed creation.
 * @param error - The mutation error.
 * @returns The message to show.
 */
function createErrorText(error: unknown): string {
	if (error instanceof EmailTakenError) {
		return __('That email is already in use.', 'alphone')
	}
	if (error instanceof ValidationError) {
		return error.message
	}
	return __('The user could not be created.', 'alphone')
}

/**
 * Renders the new user form and reports the created account upward.
 * @returns The new user screen.
 */
export function NewUserScreen({ onCreated }: { onCreated: () => void | Promise<void> }) {
	const queryClient = useQueryClient()
	const [email, setEmail] = useState('')
	const [name, setName] = useState('')
	const [password, setPassword] = useState('')
	const create = useMutation({
		mutationFn: () => createUser({ email: email.trim(), name: name.trim(), password }),
		onSuccess: async () => {
			await queryClient.invalidateQueries({ queryKey: usersQueryKey })
			await onCreated()
		},
	})

	return (
		<PageScreen title={__('New user', 'alphone')}>
			<form
				className="godmin-form"
				onSubmit={(event) => {
					event.preventDefault()
					create.mutate()
				}}
			>
				<InputControl
					label={__('Email', 'alphone')}
					type="email"
					autoComplete="off"
					value={email}
					onChange={(event) => setEmail(event.target.value)}
				/>
				<InputControl
					label={__('Name', 'alphone')}
					autoComplete="off"
					value={name}
					onChange={(event) => setName(event.target.value)}
				/>
				<InputControl
					label={__('Password', 'alphone')}
					type="password"
					autoComplete="new-password"
					value={password}
					onChange={(event) => setPassword(event.target.value)}
				/>
				<Button
					type="submit"
					disabled={
						email.trim() === '' || name.trim() === '' || password === '' || create.isPending
					}
				>
					{__('Create user', 'alphone')}
				</Button>
				{create.isError ? <ErrorNotice>{createErrorText(create.error)}</ErrorNotice> : null}
			</form>
		</PageScreen>
	)
}
