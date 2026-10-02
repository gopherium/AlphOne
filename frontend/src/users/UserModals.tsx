// SPDX-License-Identifier: AGPL-3.0-or-later

import {
	Button,
	ErrorNotice,
	InputControl,
	SelectControl,
	Stack,
	Text,
	__,
	graphError,
	sprintf,
	useGraphMutation,
	useToaster,
	validationMessage,
} from '@alphone/frontend-sdk'
import { useEnableWpCompatOverlaySlot } from '@alphone/frontend-sdk/dataviews'
import type { RenderModalProps } from '@alphone/frontend-sdk/dataviews'
import { usersQueryKey } from '@gopherium/react-auth/admin'
import { useQueryClient } from '@tanstack/react-query'
import { useState } from 'react'

import type { Account } from '../auth/graphTransport'
import { resendInviteMutation, setUserRoleMutation } from '../auth/operations'
import { roleLabel } from './userFields'

/**
 * Renders the modal writing the role another account holds.
 * @param props - The account, the handler closing the modal, and the roles the reader may grant.
 * @returns The role form.
 */
export function RoleModal({
	items: [user],
	closeModal,
	grantable,
}: RenderModalProps<Account> & { grantable: string[] }) {
	useEnableWpCompatOverlaySlot()
	const queryClient = useQueryClient()
	const toaster = useToaster()
	const [chosen, setChosen] = useState(user.role)
	const [written, write] = useGraphMutation(setUserRoleMutation)
	const offered = grantable.map((role) => ({ value: role, label: roleLabel(role) }))
	const submit = async () => {
		const result = await write({ id: user.id, role: chosen })
		if (result.error) {
			return
		}
		await queryClient.invalidateQueries({ queryKey: usersQueryKey })
		toaster.show(__('Role changed.', 'alphone'))
		closeModal?.()
	}

	return (
		<form
			onSubmit={(event) => {
				event.preventDefault()
				void submit()
			}}
		>
			<Stack direction="column" gap="lg">
				<Text>{sprintf(__('Choose the role of %(name)s.', 'alphone'), { name: user.name })}</Text>
				<SelectControl
					label={__('Role', 'alphone')}
					items={offered}
					value={offered.find((option) => option.value === chosen)}
					onValueChange={(item) => item?.value != null && setChosen(item.value)}
				/>
				{written.error ? (
					<ErrorNotice>
						{validationMessage(graphError(written.error), __('The role could not be changed.', 'alphone'))}
					</ErrorNotice>
				) : null}
				<Stack direction="row" gap="sm" justify="flex-end">
					<Button variant="minimal" onClick={closeModal}>
						{__('Cancel', 'alphone')}
					</Button>
					<Button type="submit" loading={written.fetching} disabled={chosen === user.role}>
						{__('Change role', 'alphone')}
					</Button>
				</Stack>
			</Stack>
		</form>
	)
}

/**
 * Renders the modal resending an invitation, showing the link to copy when no mail server delivered it.
 * @param props - The account and the handler closing the modal.
 * @returns The resend confirmation, or the link once handed back.
 */
export function ResendModal({ items: [user], closeModal }: RenderModalProps<Account>) {
	const toaster = useToaster()
	const [sent, send] = useGraphMutation(resendInviteMutation)
	const link = sent.data?.resendInvite.activationLink
	const submit = async () => {
		const result = await send({ email: user.email })
		if (result.data?.resendInvite.delivered) {
			toaster.show(__('Invitation sent.', 'alphone'))
			closeModal?.()
		}
	}

	if (link) {
		return (
			<Stack direction="column" gap="lg">
				<Text>{__('No mail server is configured. Deliver the activation link by hand.', 'alphone')}</Text>
				<InputControl label={__('Activation link', 'alphone')} readOnly value={link} />
				<Stack direction="row" justify="flex-end">
					<Button onClick={closeModal}>{__('Done', 'alphone')}</Button>
				</Stack>
			</Stack>
		)
	}
	return (
		<Stack direction="column" gap="lg">
			<Text>
				{sprintf(
					__('Send a new invitation to %(name)s (%(email)s)? The link sent before stops working.', 'alphone'),
					{ name: user.name, email: user.email },
				)}
			</Text>
			{sent.error ? (
				<ErrorNotice>
					{validationMessage(graphError(sent.error), __('The invitation could not be sent.', 'alphone'))}
				</ErrorNotice>
			) : null}
			<Stack direction="row" gap="sm" justify="flex-end">
				<Button variant="minimal" onClick={closeModal}>
					{__('Cancel', 'alphone')}
				</Button>
				<Button loading={sent.fetching} onClick={() => void submit()}>
					{__('Resend invitation', 'alphone')}
				</Button>
			</Stack>
		</Stack>
	)
}
