// SPDX-License-Identifier: AGPL-3.0-or-later

import { expect, test } from 'vitest'

import { runEach } from '../index'
import type { BulkFailure, BulkOutcome } from '../index'

test('hands plugins the runner making one call per item and counting the failures', async () => {
	const refused: BulkFailure<string> = { item: 'refused', error: 'not allowed' }

	const outcome: BulkOutcome<string> = await runEach(['kept', 'refused'], async (item) =>
		item === 'refused' ? { error: 'not allowed' } : { error: null },
	)

	expect(outcome).toEqual({ asked: 2, done: 1, failures: [refused] })
})
