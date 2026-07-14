// SPDX-License-Identifier: AGPL-3.0-or-later

import { Button, InputControl, Stack, Text } from '@alphone/frontend-sdk'
import { useMutation, useQueryClient } from '@tanstack/react-query'
import { useNavigate } from '@tanstack/react-router'
import { useState } from 'react'

import { EmailTakenError, ValidationError, createUser, usersQueryKey } from './api'

/**
 * Maps a creation failure to the message shown under the form, surfacing
 * the backend's explanation for rejected input.
 * @param error - The error thrown by the create mutation.
 * @returns The human-readable failure message.
 */
function createErrorMessage(error: Error): string {
	if (error instanceof EmailTakenError) {
		return 'That email is already in use.'
	}
	if (error instanceof ValidationError) {
		return error.message
	}
	return 'The user could not be created.'
}

/**
 * Renders the new user form, creating an account and returning to the
 * user list on success.
 * @returns The new user screen element.
 */
export function NewUserScreen() {
	const queryClient = useQueryClient()
	const navigate = useNavigate()
	const [email, setEmail] = useState('')
	const [name, setName] = useState('')
	const [password, setPassword] = useState('')
	const create = useMutation({
		mutationFn: () => createUser({ email: email.trim(), name: name.trim(), password }),
		onSuccess: async () => {
			await queryClient.invalidateQueries({ queryKey: usersQueryKey })
			await navigate({ to: '/users' })
		},
	})

	return (
		<Stack direction="column" gap="lg">
			<Text variant="heading-lg" render={<h1 />}>
				New user
			</Text>
			<form
				onSubmit={(event) => {
					event.preventDefault()
					create.mutate()
				}}
			>
				<Stack direction="column" gap="md">
					<InputControl
						label="Email"
						type="email"
						autoComplete="off"
						value={email}
						onChange={(event) => setEmail(event.target.value)}
					/>
					<InputControl
						label="Name"
						autoComplete="off"
						value={name}
						onChange={(event) => setName(event.target.value)}
					/>
					<InputControl
						label="Password"
						type="password"
						autoComplete="new-password"
						value={password}
						onChange={(event) => setPassword(event.target.value)}
					/>
					<Button
						type="submit"
						disabled={
							email.trim() === '' ||
							name.trim() === '' ||
							password === '' ||
							create.isPending
						}
					>
						Create user
					</Button>
					{create.isError ? (
						<Text role="alert">{createErrorMessage(create.error)}</Text>
					) : null}
				</Stack>
			</form>
		</Stack>
	)
}
