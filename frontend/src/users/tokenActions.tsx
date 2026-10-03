// SPDX-License-Identifier: AGPL-3.0-or-later

import { __, _n, trash } from '@alphone/frontend-sdk'
import type { Action } from '@alphone/frontend-sdk/dataviews'
import { useMemo } from 'react'

import { RevokeModal } from './TokenModals'
import type { RevokeHandlers } from './TokenModals'
import type { ApiToken } from './tokenFields'

/**
 * Returns the revoke the tokens list offers on every row and on the picked rows at once.
 * @param onFailure - Shows the failure of a revoke above the list, or clears it.
 * @param onRevoked - Reloads the list once a revoke ran.
 * @returns The row and bulk actions.
 */
export function useTokenActions(
	onFailure: RevokeHandlers['onFailure'],
	onRevoked: RevokeHandlers['onRevoked'],
): Action<ApiToken>[] {
	return useMemo(
		() => [
			{
				id: 'revoke',
				label: __('Revoke', 'alphone'),
				icon: trash,
				supportsBulk: true,
				modalHeader: (tokens) => _n('Revoke token', 'Revoke tokens', tokens.length, 'alphone'),
				modalSize: 'small',
				RenderModal: (props) => <RevokeModal {...props} onFailure={onFailure} onRevoked={onRevoked} />,
			},
		],
		[onFailure, onRevoked],
	)
}
