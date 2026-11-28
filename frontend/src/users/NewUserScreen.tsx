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
import { RoleSelect, addUserErrorMessageOf } from '@gopherium/react-auth/wpds/users'
import { useMutation, useQueryClient } from '@tanstack/react-query'
import { Link, useNavigate } from '@tanstack/react-router'
import { useEffect, useRef, useState } from 'react'
import type { ComponentProps } from 'react'

import { accountErrorMessages } from './errorMessages'
import { roleLabels, startingRole } from './roles'

/** FieldProps are the props of the WPDS field a required field draws. */
type FieldProps = ComponentProps<typeof ValidatedInputControl>

/** RequiredFieldProps are what a field the form needs filled takes beside the props of the WPDS field. */
type RequiredFieldProps = Omit<FieldProps, 'value' | 'required' | 'customValidity'> & {
	/** value is what the field holds. */
	value: string
	/** rejected is what the server said about the value the field holds, if anything. */
	rejected: FieldProps['customValidity']
	/** empty is the words the field shows under it while it holds nothing but spaces. */
	empty: string
}

/**
 * Renders a field the form needs filled, its words under it once the reader leaves it or sends the form.
 * @param props - The value, what the server said about it, the words for an empty field and the field's own props.
 * @returns The field.
 */
function RequiredField({ rejected, empty, ...field }: RequiredFieldProps) {
	const blank = field.value.trim() === '' ? { type: 'invalid' as const, message: empty } : undefined
	return <ValidatedInputControl {...field} required customValidity={rejected ?? blank} />
}

/**
 * Returns the words a refused invitation reads as, the server's own only for a refused value it named no code for.
 * @param error - The mutation error.
 * @returns The words, empty when the refusal has none worth showing.
 */
function refusalWords(error: unknown): string {
	const unnamed = error instanceof AccountValidationError && error.code === undefined ? error.message : ''
	return addUserErrorMessageOf(error, accountErrorMessages(), unnamed)
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
				await onDelivered()
				toaster.show(__('Invitation sent.', 'alphone'))
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
			<RequiredField
				label={__('Email', 'alphone')}
				type="email"
				autoComplete="off"
				value={email}
				rejected={refused.email.validity}
				empty={__('Please enter an email address.', 'alphone')}
				onChange={(event) => setEmail(event.target.value)}
			/>
			<RequiredField
				label={__('Name', 'alphone')}
				autoComplete="off"
				value={name}
				rejected={refused.name.validity}
				empty={__('Please provide a name.', 'alphone')}
				onChange={(event) => setName(event.target.value)}
			/>
			<RoleSelect grantable={grantable} value={role} onChange={setRole} roleLabels={roleLabels()} />
			<Button type="submit" loading={send.isPending}>
				{__('Send invitation', 'alphone')}
			</Button>
		</form>
	)
}

/**
 * Renders the activation link to deliver by hand in place of the form, the focus on its field.
 * @param props - The link, and what runs once the reader is done with it.
 * @returns The link and the way back to the list.
 */
function ActivationLink({
	link,
	onDone,
}: {
	link: string
	onDone: () => void | Promise<void>
}) {
	const field = useRef<HTMLInputElement>(null)
	useEffect(() => {
		field.current?.focus()
	}, [])

	return (
		<div className="godmin-form">
			<Text role="status">
				{__('No mail server is configured. Deliver the activation link by hand.', 'alphone')}
			</Text>
			<InputControl ref={field} label={__('Activation link', 'alphone')} readOnly value={link} />
			<Button onClick={() => onDone()}>{__('Done', 'alphone')}</Button>
		</div>
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
			title={__('Add User', 'alphone')}
			subtitle={__('Create a brand new user and add them to this workspace.', 'alphone')}
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
				<ActivationLink link={activationLink} onDone={onCreated} />
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
