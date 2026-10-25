// SPDX-License-Identifier: AGPL-3.0-or-later

import {
	__,
	_n,
	check,
	formatNumber,
	graphError,
	notAllowed,
	sprintf,
	useGraph,
	useToaster,
	validationMessage,
} from '@alphone/frontend-sdk'
import type { BulkFailure, BulkRun, BulkWords, Session } from '@alphone/frontend-sdk'
import type { Action } from '@alphone/frontend-sdk/dataviews'
import { useMemo } from 'react'

import type { Account } from '../auth/graphTransport'
import { setUserDisabledMutation } from '../auth/operations'
import { ResendModal, RoleModal } from './UserModals'

/**
 * Returns the toast confirming the accounts one disable or enable changed.
 * @param disabled - Whether the accounts were disabled rather than enabled.
 * @param done - How many accounts changed.
 * @param only - The one account asked, when only one was.
 * @param cut - Cuts a name to the length a toast shows.
 * @returns The confirmation, naming the account when only one was asked.
 */
function switchedMessage(
	disabled: boolean,
	done: number,
	only: Account | undefined,
	cut: (name: string) => string,
): string {
	if (only !== undefined) {
		const template = disabled ? __('"%(name)s" disabled.', 'alphone') : __('"%(name)s" enabled.', 'alphone')
		return sprintf(template, { name: cut(only.name) })
	}
	const template = disabled
		? _n('%s user disabled.', '%s users disabled.', done, 'alphone')
		: _n('%s user enabled.', '%s users enabled.', done, 'alphone')
	return sprintf(template, formatNumber(done))
}

/**
 * Returns the toast naming the accounts one disable or enable could not change.
 * @param disabled - Whether the accounts were to be disabled rather than enabled.
 * @param asked - How many accounts the action was asked to change.
 * @param failures - Each unchanged account with the failure it answered with.
 * @returns The reason for one account, the count for several.
 */
function unswitchedMessage(disabled: boolean, asked: number, failures: BulkFailure<Account>[]): string {
	if (asked === 1) {
		const fallback = disabled
			? __('The user could not be disabled.', 'alphone')
			: __('The user could not be enabled.', 'alphone')
		return validationMessage(graphError(failures[0].error), fallback)
	}
	const template = disabled
		? _n('%s user could not be disabled.', '%s users could not be disabled.', failures.length, 'alphone')
		: _n('%s user could not be enabled.', '%s users could not be enabled.', failures.length, 'alphone')
	return sprintf(template, formatNumber(failures.length))
}

/**
 * Returns the words a disable or enable run reports its outcome in.
 * @param disabled - Whether the run disables the accounts rather than enables them.
 * @param cut - Cuts a name to the length a toast shows.
 * @returns The words for the accounts changed and for the ones that failed.
 */
function switchWords(disabled: boolean, cut: (name: string) => string): BulkWords<Account> {
	return {
		done: (done, only) => switchedMessage(disabled, done, only, cut),
		failed: (failures, asked) => unswitchedMessage(disabled, asked, failures),
	}
}

/**
 * Returns the users list actions, none on the reader's own row, a role or standing change only within reach.
 * @param session - The signed-in account.
 * @param bulk - Runs a disable or enable over the picked accounts, reloads the list and shows one toast.
 * @returns The row and bulk actions, none once the session is gone.
 */
export function useUserActions(session: Session | null, bulk: BulkRun<Account>): Action<Account>[] {
	const graph = useGraph()
	const toaster = useToaster()

	return useMemo(() => {
		if (session === null) {
			return []
		}
		const changeable = (user: Account) => user.id !== session.id && user.withinReach
		const switchAccounts = (items: Account[], disabled: boolean) =>
			bulk(
				items,
				(user) => graph.client.mutation(setUserDisabledMutation, { id: user.id, disabled }).toPromise(),
				switchWords(disabled, (name) => toaster.name(name)),
			)
		const actions: Action<Account>[] = [
			{
				id: 'change-role',
				label: __('Change role…', 'alphone'),
				modalHeader: __('Change role', 'alphone'),
				modalSize: 'small',
				isEligible: changeable,
				RenderModal: (props) => <RoleModal {...props} grantable={session.grantable} />,
			},
			{
				id: 'resend-invitation',
				label: __('Resend invitation…', 'alphone'),
				modalHeader: __('Resend invitation', 'alphone'),
				modalFocusOnMount: 'firstContentElement',
				isEligible: (user) => !user.confirmed && !user.disabled,
				RenderModal: ResendModal,
			},
			{
				id: 'enable',
				label: __('Enable', 'alphone'),
				icon: check,
				isPrimary: true,
				supportsBulk: true,
				isEligible: (user) => changeable(user) && user.disabled && session.grantable.includes(user.role),
				callback: (items) => switchAccounts(items, false),
			},
			{
				id: 'disable',
				label: __('Disable', 'alphone'),
				icon: notAllowed,
				supportsBulk: true,
				isEligible: (user) => changeable(user) && !user.disabled,
				callback: (items) => switchAccounts(items, true),
			},
		]
		return actions
	}, [session, graph, toaster, bulk])
}
