// SPDX-License-Identifier: AGPL-3.0-or-later

import * as gottext from '@gopherium/gottext'
import { expect, test } from 'vitest'

import {
	formatDate,
	formatList,
	formatMoney,
	formatNumber,
	formatTime,
	formatWeekday,
	rememberFormatLocale,
} from '../index'

test('hands plugins the format functions the admin writes in, the very ones gottext ships', () => {
	const handed = [formatDate, formatList, formatMoney, formatNumber, formatTime, formatWeekday, rememberFormatLocale]

	expect(handed).toEqual([
		gottext.formatDate,
		gottext.formatList,
		gottext.formatMoney,
		gottext.formatNumber,
		gottext.formatTime,
		gottext.formatWeekday,
		gottext.rememberFormatLocale,
	])
	expect(handed.map((piece) => typeof piece)).toEqual(handed.map(() => 'function'))
})
