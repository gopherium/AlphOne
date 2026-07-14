// SPDX-License-Identifier: AGPL-3.0-or-later

import { Button, InputControl, Stack, Text } from '@alphone/frontend-sdk'
import { useMutation, useQueryClient } from '@tanstack/react-query'
import { useNavigate } from '@tanstack/react-router'
import { useState } from 'react'

import { EmailTakenError, createUser } from './api'

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
			await queryClient.invalidateQueries({ queryKey: ['users'] })
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
						value={email}
						onChange={(event) => setEmail(event.target.value)}
					/>
					<InputControl
						label="Name"
						value={name}
						onChange={(event) => setName(event.target.value)}
					/>
					<InputControl
						label="Password"
						type="password"
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
						<Text role="alert">
							{create.error instanceof EmailTakenError
								? 'That email is already in use.'
								: 'The user could not be created.'}
						</Text>
					) : null}
				</Stack>
			</form>
		</Stack>
	)
}
