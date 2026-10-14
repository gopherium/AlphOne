// SPDX-License-Identifier: AGPL-3.0-or-later

import {
	ConfirmBody,
	__,
	_n,
	formatNumber,
	graphError,
	runEach,
	sprintf,
	useGraph,
	useToaster,
	validationMessage,
} from '@alphone/frontend-sdk'
import type { BulkOutcome } from '@alphone/frontend-sdk'
import type { RenderModalProps } from '@alphone/frontend-sdk/dataviews'
import { useState } from 'react'

import type { ApiToken } from './tokenFields'
import { apiTokenRevokeMutation } from './tokenOperations'

/** RevokeOutcome reports the tokens one revoke reached and the failures of the rest. */
type RevokeOutcome = BulkOutcome<ApiToken>

/** RevokeHandlers carries what the screen does once a revoke ran. */
export interface RevokeHandlers {
	onFailure: (message: string | undefined) => void
	onRevoked: () => void
}

/**
 * Returns the question the revoke modal asks.
 * @param tokens - The tokens the reader picked.
 * @param name - The name of the first token, cut to the length a toast shows.
 * @returns The question, naming the token when only one was picked.
 */
function revokeQuestion(tokens: ApiToken[], name: string): string {
	if (tokens.length === 1) {
		return sprintf(__('Revoke "%(name)s"? This cannot be undone.', 'alphone'), { name })
	}
	const many = _n(
		'Revoke %s token? This cannot be undone.',
		'Revoke %s tokens? This cannot be undone.',
		tokens.length,
		'alphone',
	)
	return sprintf(many, formatNumber(tokens.length))
}

/**
 * Returns the toast confirming the tokens one revoke reached.
 * @param outcome - What the revoke reached.
 * @param name - The name of the first token, cut to the length a toast shows.
 * @returns The confirmation, naming the token when only one was asked.
 */
function revokedMessage({ asked, done }: RevokeOutcome, name: string): string {
	if (asked === 1) {
		return sprintf(__('"%(name)s" revoked.', 'alphone'), { name })
	}
	return sprintf(_n('%s token revoked.', '%s tokens revoked.', done, 'alphone'), formatNumber(done))
}

/**
 * Returns the notice naming the tokens one revoke could not reach.
 * @param outcome - What the revoke reached.
 * @returns The reason for one token, the count for several.
 */
function unrevokedMessage({ asked, failures }: RevokeOutcome): string {
	if (asked === 1) {
		const reason = graphError(failures[0].error)
		return validationMessage(reason, __('The token could not be revoked.', 'alphone'))
	}
	const template = _n('%s token could not be revoked.', '%s tokens could not be revoked.', failures.length, 'alphone')
	return sprintf(template, formatNumber(failures.length))
}

/**
 * Renders the modal asking before the picked tokens are revoked, then revokes them one call each.
 * @param props - The tokens, the handler closing the modal, and what the screen does once they ran.
 * @returns The confirmation.
 */
export function RevokeModal({ items, closeModal, onFailure, onRevoked }: RenderModalProps<ApiToken> & RevokeHandlers) {
	const graph = useGraph()
	const toaster = useToaster()
	const [busy, setBusy] = useState(false)
	const name = toaster.name(items[0].name)
	const submit = async () => {
		setBusy(true)
		onFailure(undefined)
		const outcome = await runEach(items, (token) =>
			graph.client.mutation(apiTokenRevokeMutation, { id: token.id }).toPromise(),
		)
		onRevoked()
		if (outcome.done > 0) {
			toaster.show(revokedMessage(outcome, name))
		}
		if (outcome.failures.length > 0) {
			onFailure(unrevokedMessage(outcome))
		}
		closeModal?.()
	}

	return (
		<ConfirmBody
			confirmLabel={__('Revoke', 'alphone')}
			cancelLabel={__('Cancel', 'alphone')}
			busy={busy}
			onConfirm={() => void submit()}
			onCancel={busy ? undefined : closeModal}
		>
			{revokeQuestion(items, name)}
		</ConfirmBody>
	)
}
