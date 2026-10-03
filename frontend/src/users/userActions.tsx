// SPDX-License-Identifier: AGPL-3.0-or-later

import {
	MANAGE_USERS,
	__,
	_n,
	can,
	check,
	formatNumber,
	graphError,
	notAllowed,
	sprintf,
	useGraph,
	useToaster,
	validationMessage,
} from '@alphone/frontend-sdk'
import type { GraphFailure, Session } from '@alphone/frontend-sdk'
import type { Action } from '@alphone/frontend-sdk/dataviews'
import { usersQueryKey } from '@gopherium/react-auth/admin'
import { useQueryClient } from '@tanstack/react-query'
import { useMemo } from 'react'

import type { Account } from '../auth/graphTransport'
import { setUserDisabledMutation } from '../auth/operations'
import { ResendModal, RoleModal } from './UserModals'

/**
 * Returns the toast confirming the accounts one disable or enable changed.
 * @param disabled - Whether the accounts were disabled rather than enabled.
 * @param asked - How many accounts the action was asked to change.
 * @param done - How many of them changed.
 * @returns The confirmation.
 */
function switchedMessage(disabled: boolean, asked: number, done: number): string {
	if (asked === 1) {
		return disabled ? __('User disabled.', 'alphone') : __('User enabled.', 'alphone')
	}
	const template = disabled
		? _n('%s user disabled.', '%s users disabled.', done, 'alphone')
		: _n('%s user enabled.', '%s users enabled.', done, 'alphone')
	return sprintf(template, formatNumber(done))
}

/**
 * Returns the notice naming the accounts one disable or enable could not change.
 * @param disabled - Whether the accounts were to be disabled rather than enabled.
 * @param asked - How many accounts the action was asked to change.
 * @param failures - The failure each unchanged account answered with.
 * @returns The reason for one account, the count for several.
 */
function unswitchedMessage(disabled: boolean, asked: number, failures: GraphFailure[]): string {
	if (asked === 1) {
		const fallback = disabled
			? __('The user could not be disabled.', 'alphone')
			: __('The user could not be enabled.', 'alphone')
		return validationMessage(graphError(failures[0]), fallback)
	}
	const template = disabled
		? _n('%s user could not be disabled.', '%s users could not be disabled.', failures.length, 'alphone')
		: _n('%s user could not be enabled.', '%s users could not be enabled.', failures.length, 'alphone')
	return sprintf(template, formatNumber(failures.length))
}

/**
 * Returns the actions the users list offers a reader who manages users, none on their own row.
 * @param session - The signed-in account.
 * @param onFailure - Shows the failure of a disable or enable above the list, or clears it.
 * @returns The row and bulk actions, none for a reader who may not manage users.
 */
export function useUserActions(
	session: Session | null,
	onFailure: (message: string | undefined) => void,
): Action<Account>[] {
	const graph = useGraph()
	const toaster = useToaster()
	const queryClient = useQueryClient()

	return useMemo(() => {
		if (session === null || !can(session, MANAGE_USERS)) {
			return []
		}
		const others = (user: Account) => user.id !== session.id
		const switchAccounts = async (items: Account[], disabled: boolean) => {
			onFailure(undefined)
			const results = await Promise.all(
				items.map((user) => graph.client.mutation(setUserDisabledMutation, { id: user.id, disabled }).toPromise()),
			)
			const failures = results.flatMap((result) => (result.error ? [result.error] : []))
			const done = items.length - failures.length
			await queryClient.invalidateQueries({ queryKey: usersQueryKey })
			if (done > 0) {
				toaster.show(switchedMessage(disabled, items.length, done))
			}
			if (failures.length > 0) {
				onFailure(unswitchedMessage(disabled, items.length, failures))
			}
		}
		const actions: Action<Account>[] = [
			{
				id: 'change-role',
				label: __('Change role', 'alphone'),
				modalHeader: __('Change role', 'alphone'),
				modalSize: 'small',
				isEligible: others,
				RenderModal: (props) => <RoleModal {...props} grantable={session.grantable} />,
			},
			{
				id: 'resend-invitation',
				label: __('Resend invitation', 'alphone'),
				modalHeader: __('Resend invitation', 'alphone'),
				modalSize: 'small',
				isEligible: (user) => !user.confirmed && !user.disabled,
				RenderModal: ResendModal,
			},
			{
				id: 'enable',
				label: __('Enable', 'alphone'),
				icon: check,
				supportsBulk: true,
				isEligible: (user) => others(user) && user.disabled,
				callback: (items) => switchAccounts(items, false),
			},
			{
				id: 'disable',
				label: __('Disable', 'alphone'),
				icon: notAllowed,
				supportsBulk: true,
				isEligible: (user) => others(user) && !user.disabled,
				callback: (items) => switchAccounts(items, true),
			},
		]
		return actions
	}, [session, graph, toaster, queryClient, onFailure])
}
