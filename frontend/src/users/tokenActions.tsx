// SPDX-License-Identifier: AGPL-3.0-or-later

import { __, trash } from '@alphone/frontend-sdk'
import type { BulkRun } from '@alphone/frontend-sdk'
import type { Action } from '@alphone/frontend-sdk/dataviews'
import { useMemo } from 'react'

import { RevokeModal } from './TokenModals'
import type { ApiToken } from './tokenFields'

/**
 * Returns the revoke the tokens list offers on every row and on the picked rows at once.
 * @param bulk - Runs the revoke over the picked tokens, reloads the list and shows one toast.
 * @returns The row and bulk actions.
 */
export function useTokenActions(bulk: BulkRun<ApiToken>): Action<ApiToken>[] {
	return useMemo(
		() => [
			{
				id: 'revoke',
				label: __('Revoke…', 'alphone'),
				icon: trash,
				supportsBulk: true,
				hideModalHeader: true,
				modalFocusOnMount: 'firstContentElement',
				RenderModal: (props) => <RevokeModal {...props} bulk={bulk} />,
			},
		],
		[bulk],
	)
}
