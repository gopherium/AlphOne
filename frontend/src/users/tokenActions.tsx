// SPDX-License-Identifier: AGPL-3.0-or-later

import { __, trash } from '@alphone/frontend-sdk'
import type { Action } from '@alphone/frontend-sdk/dataviews'
import { useMemo } from 'react'

import type { ApiToken } from './tokenFields'

/**
 * Returns the revoke the tokens list offers on every row and on the picked rows at once.
 * @param onRevoke - Stores the tokens the reader picked to revoke.
 * @returns The row and bulk actions.
 */
export function useTokenActions(onRevoke: (tokens: ApiToken[]) => void): Action<ApiToken>[] {
	return useMemo(
		() => [{ id: 'revoke', label: __('Revoke', 'alphone'), icon: trash, supportsBulk: true, callback: onRevoke }],
		[onRevoke],
	)
}
