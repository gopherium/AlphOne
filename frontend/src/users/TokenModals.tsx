// SPDX-License-Identifier: AGPL-3.0-or-later

import {
	RunConfirm,
	__,
	_n,
	formatNumber,
	graphError,
	sprintf,
	useGraph,
	validationMessage,
} from '@alphone/frontend-sdk'
import type { BulkFailure, BulkRun, BulkWords } from '@alphone/frontend-sdk'
import type { RenderModalProps } from '@alphone/frontend-sdk/dataviews'

import type { ApiToken } from './tokenFields'
import { apiTokenRevokeMutation } from './tokenOperations'

/**
 * Returns the question the revoke modal asks.
 * @param tokens - The tokens the reader picked.
 * @returns The question, naming the token when only one was picked.
 */
function revokeQuestion(tokens: ApiToken[]): string {
	if (tokens.length === 1) {
		return sprintf(__('Revoke "%(name)s"? This cannot be undone.', 'alphone'), { name: tokens[0].name })
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
 * @param done - How many tokens the revoke reached.
 * @param only - The one token asked, when only one was.
 * @returns The confirmation, naming the token when only one was asked.
 */
function revokedMessage(done: number, only: ApiToken | undefined): string {
	if (only !== undefined) {
		return sprintf(__('"%(name)s" revoked.', 'alphone'), { name: only.name })
	}
	return sprintf(_n('%s token revoked.', '%s tokens revoked.', done, 'alphone'), formatNumber(done))
}

/**
 * Returns the toast naming the tokens one revoke could not reach.
 * @param failures - The tokens the revoke could not reach, each with its error.
 * @param asked - How many tokens the revoke was asked to reach.
 * @returns The reason for one token, the count for several.
 */
function unrevokedMessage(failures: BulkFailure<ApiToken>[], asked: number): string {
	if (asked === 1) {
		const reason = graphError(failures[0].error)
		return validationMessage(reason, __('The token could not be revoked.', 'alphone'))
	}
	const template = _n('%s token could not be revoked.', '%s tokens could not be revoked.', failures.length, 'alphone')
	return sprintf(template, formatNumber(failures.length))
}

/** The words a revoke run reports its outcome in. */
const revokeWords: BulkWords<ApiToken> = {
	done: revokedMessage,
	failed: unrevokedMessage,
}

/**
 * Renders the modal asking before the picked tokens are revoked, then revokes them one call each.
 * @param props - The tokens, the handler closing the modal, and the run the list reports through.
 * @returns The confirmation.
 */
export function RevokeModal({ items, closeModal, bulk }: RenderModalProps<ApiToken> & { bulk: BulkRun<ApiToken> }) {
	const graph = useGraph()

	return (
		<RunConfirm
			items={items}
			closeModal={closeModal}
			confirmLabel={__('Revoke', 'alphone')}
			cancelLabel={__('Cancel', 'alphone')}
			run={(tokens) =>
				bulk(
					tokens,
					(token) => graph.client.mutation(apiTokenRevokeMutation, { id: token.id }).toPromise(),
					revokeWords,
				)
			}
		>
			{revokeQuestion(items)}
		</RunConfirm>
	)
}
