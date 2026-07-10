// SPDX-License-Identifier: AGPL-3.0-or-later

import { Button, Card, InputControl, Stack, Text } from '@alphone/frontend-sdk'
import { useMutation } from '@tanstack/react-query'
import { useState } from 'react'

import { InvalidCredentialsError, login } from './api'
import type { User } from './api'

/**
 * Renders the login form and reports the authenticated user upward.
 * @param onLogin - Called with the user after a successful login.
 * @returns The login screen element.
 */
export function LoginScreen({
	onLogin,
}: {
	onLogin: (user: User) => void | Promise<void>
}) {
	const [email, setEmail] = useState('')
	const [password, setPassword] = useState('')
	const attempt = useMutation({
		mutationFn: () => login(email.trim(), password),
		onSuccess: (user) => onLogin(user),
	})

	return (
		<div className="alphone-login">
			<Card.Root className="alphone-login__card">
				<Card.Content>
					<form
						onSubmit={(event) => {
							event.preventDefault()
							attempt.mutate()
						}}
					>
						<Stack direction="column" gap="lg">
							<Text variant="heading-lg" render={<h1 />}>
								AlphOne
							</Text>
							<InputControl
								label="Email"
								type="email"
								value={email}
								onChange={(event) => setEmail(event.target.value)}
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
									email.trim() === '' || password === '' || attempt.isPending
								}
							>
								Log in
							</Button>
							{attempt.isError ? (
								<Text role="alert">
									{attempt.error instanceof InvalidCredentialsError
										? 'Invalid email or password.'
										: 'Login failed, please try again.'}
								</Text>
							) : null}
						</Stack>
					</form>
				</Card.Content>
			</Card.Root>
		</div>
	)
}
