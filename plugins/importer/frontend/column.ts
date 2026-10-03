// SPDX-License-Identifier: AGPL-3.0-or-later

import { __, formatNumber, sprintf } from '@alphone/frontend-sdk'

/**
 * Returns the name a column without a header goes by.
 * @param index - The place of the column in the file, from zero.
 * @returns The name, such as Column 2.
 */
export function columnLabel(index: number): string {
	return sprintf(__('Column %(number)s', 'alphone-importer'), { number: formatNumber(index + 1) })
}
