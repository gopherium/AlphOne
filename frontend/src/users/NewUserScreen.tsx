// SPDX-License-Identifier: AGPL-3.0-or-later

import {
	Button,
	InputControl,
	PageBreadcrumb,
	PageBreadcrumbs,
	PageScreen,
	Text,
	ValidatedInputControl,
	ValidationError,
	__,
	useRefusedField,
	useSession,
	useToaster,
	writeFailure,
} from '@alphone/frontend-sdk'
import {
	ValidationError as AccountValidationError,
	fieldError,
	invite,
	usersQueryKey,
} from '@gopherium/react-auth/admin'
import type { NewInvite } from '@gopherium/react-auth/admin'
import { RoleSelect, errorMessageOf } from '@gopherium/react-auth/wpds/users'
import { useMutation, useQueryClient } from '@tanstack/react-query'
import { Link, useNavigate } from '@tanstack/react-router'
import { useRef, useState } from 'react'

import { accountErrorMessages } from './errorMessages'
import { roleLabels, startingRole } from './roles'

/**
 * Returns the words a refused invitation reads as, the server's own only for a refused value it named no code for.
 * @param error - The mutation error.
 * @returns The words, empty when the refusal has none worth showing.
 */
function refusalWords(error: unknown): string {
	const unnamed = error instanceof AccountValidationError && error.code === undefined ? error.message : ''
	return errorMessageOf(error, accountErrorMessages(), unnamed)
}

/**
 * Returns a failed invitation as the validation error the SDK reads, in the words the reader is told.
 * @param error - The mutation error.
 * @returns The validation error, or nothing when the failure has no words worth showing.
 */
function inviteFailure(error: unknown): ValidationError | undefined {
	const words = fieldError(error)?.message ?? refusalWords(error)
	return words === '' ? undefined : new ValidationError(words)
}

/**
 * Returns the box of the form a failed invitation is about.
 * @param error - The mutation error.
 * @returns The box, or nothing when the failure is about no box the form holds.
 */
function inviteBox(error: unknown): 'email' | 'name' | undefined {
	const about = fieldError(error)?.field
	return about === 'email' || about === 'name' ? about : undefined
}

/**
 * Renders the invitation form.
 * @param props - What runs when the mail went out, and what runs with the link when no mail server delivered it.
 * @returns The form.
 */
function InvitationForm({
	onDelivered,
	onUndelivered,
}: {
	onDelivered: () => void | Promise<void>
	onUndelivered: (activationLink: string) => void
}) {
	const queryClient = useQueryClient()
	const toaster = useToaster()
	const grantable = useSession()?.grantable ?? []
	const [email, setEmail] = useState('')
	const [name, setName] = useState('')
	const [role, setRole] = useState(() => startingRole(grantable))
	const form = useRef<HTMLFormElement>(null)
	const refused = { email: useRefusedField(form, email), name: useRefusedField(form, name) }
	const send = useMutation({
		mutationFn: (sent: NewInvite) => invite({ ...sent, email: sent.email.trim(), name: sent.name.trim() }),
		onError: (error, sent) => {
			const box = inviteBox(error)
			const failure = inviteFailure(error)
			if (box === undefined || !refused[box].refuse(failure, sent[box])) {
				toaster.show(writeFailure(__('Could not send the invitation.', 'alphone'), failure))
			}
		},
		onSuccess: async (invitation) => {
			await queryClient.invalidateQueries({ queryKey: usersQueryKey })
			if (invitation.delivered) {
				toaster.show(__('Invitation sent.', 'alphone'))
				await onDelivered()
				return
			}
			onUndelivered(invitation.activation_link)
		},
	})

	return (
		<form
			ref={form}
			className="godmin-form"
			onSubmit={(event) => {
				event.preventDefault()
				send.mutate({ email, name, role })
			}}
		>
			<ValidatedInputControl
				label={__('Email', 'alphone')}
				type="email"
				autoComplete="off"
				value={email}
				customValidity={refused.email.validity}
				onChange={(event) => setEmail(event.target.value)}
			/>
			<ValidatedInputControl
				label={__('Name', 'alphone')}
				autoComplete="off"
				value={name}
				customValidity={refused.name.validity}
				onChange={(event) => setName(event.target.value)}
			/>
			<RoleSelect grantable={grantable} value={role} onChange={setRole} roleLabels={roleLabels()} />
			<Button
				type="submit"
				disabled={email.trim() === '' || name.trim() === '' || send.isPending}
			>
				{__('Send invitation', 'alphone')}
			</Button>
		</form>
	)
}

/**
 * Renders the invitation form, reporting the handled invitation upward or
 * showing the activation link when no mail server delivered it.
 * @param onCreated - Called once the invitation is handled.
 * @returns The new user screen.
 */
function NewUserScreen({ onCreated }: { onCreated: () => void | Promise<void> }) {
	const [activationLink, setActivationLink] = useState<string | null>(null)

	return (
		<PageScreen
			title={__('New user', 'alphone')}
			breadcrumbs={
				<PageBreadcrumbs label={__('Breadcrumbs', 'alphone')}>
					<PageBreadcrumb render={<Link to="/users" activeOptions={{ exact: true }} />}>
						{__('Users', 'alphone')}
					</PageBreadcrumb>
				</PageBreadcrumbs>
			}
			headingLevel={2}
		>
			{activationLink === null ? (
				<InvitationForm onDelivered={onCreated} onUndelivered={setActivationLink} />
			) : (
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
			)}
		</PageScreen>
	)
}

/**
 * Renders the new user form, returning to the user list on success.
 * @returns The new user screen element.
 */
export function NewUserRoute() {
	const navigate = useNavigate()
	return <NewUserScreen onCreated={() => navigate({ to: '/users' })} />
}
