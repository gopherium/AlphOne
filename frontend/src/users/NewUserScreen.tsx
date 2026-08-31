// SPDX-License-Identifier: AGPL-3.0-or-later

import { Button, ErrorNotice, InputControl, PageScreen, Text, __ } from '@alphone/frontend-sdk'
import { ValidationError, invite, usersQueryKey } from '@gopherium/react-auth/admin'
import { useMutation, useQueryClient } from '@tanstack/react-query'
import { useState } from 'react'

/**
 * Returns the message shown under the form for a failed invitation.
 * @param error - The mutation error.
 * @returns The message to show.
 */
function inviteErrorText(error: unknown): string {
	if (error instanceof ValidationError) {
		return error.message
	}
	return __('The invitation could not be sent.', 'alphone')
}

/**
 * Renders the invitation form, reporting the handled invitation upward or
 * showing the activation link when no mail server delivered it.
 * @param onCreated - Called once the invitation is handled.
 * @returns The new user screen.
 */
export function NewUserScreen({ onCreated }: { onCreated: () => void | Promise<void> }) {
	const queryClient = useQueryClient()
	const [email, setEmail] = useState('')
	const [name, setName] = useState('')
	const [activationLink, setActivationLink] = useState<string | null>(null)
	const send = useMutation({
		mutationFn: () => invite({ email: email.trim(), name: name.trim() }),
		onSuccess: async (invitation) => {
			await queryClient.invalidateQueries({ queryKey: usersQueryKey })
			if (invitation.delivered) {
				await onCreated()
				return
			}
			setActivationLink(invitation.activation_link)
		},
	})

	if (activationLink !== null) {
		return (
			<PageScreen title={__('New user', 'alphone')}>
				<div className="godmin-form">
					<Text role="status">
						{__('No mail server is configured. Deliver the activation link by hand.', 'alphone')}
					</Text>
					<InputControl
						label={__('Activation link', 'alphone')}
						readOnly
						value={activationLink}
					/>
					<Button onClick={() => onCreated()}>{__('Done', 'alphone')}</Button>
				</div>
			</PageScreen>
		)
	}
	return (
		<PageScreen title={__('New user', 'alphone')}>
			<form
				className="godmin-form"
				onSubmit={(event) => {
					event.preventDefault()
					send.mutate()
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
				<Button
					type="submit"
					disabled={email.trim() === '' || name.trim() === '' || send.isPending}
				>
					{__('Send invitation', 'alphone')}
				</Button>
				{send.isError ? <ErrorNotice>{inviteErrorText(send.error)}</ErrorNotice> : null}
			</form>
		</PageScreen>
	)
}
