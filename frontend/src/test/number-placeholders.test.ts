// SPDX-License-Identifier: AGPL-3.0-or-later

import { pot } from '@gopherium/gottext/build'
import { expect, test } from 'vitest'

import { domains, potConfig } from '../../scripts/config.ts'

/** NUMBER_PLACEHOLDER matches a placeholder that writes a bare number instead of formatted text. */
const NUMBER_PLACEHOLDER = /%(?:\(\w+\))?[ +0#-]*\d*(?:\.\d+)?[dfi]/

/**
 * Returns every message of one domain that writes a bare number.
 * @param domain - The domain to extract.
 * @returns The lines naming such a message.
 */
function bareNumbers(domain: ReturnType<typeof domains>[number]): string[] {
	return pot(potConfig(domain))
		.toString('utf8')
		.split('\n')
		.filter((line) => line.startsWith('msgid') || line.startsWith('"'))
		.filter((line) => NUMBER_PLACEHOLDER.test(line))
}

test('takes every number a message shows as text the format locale wrote', () => {
	for (const domain of domains()) {
		expect({ domain: domain.name, bare: bareNumbers(domain) }).toEqual({ domain: domain.name, bare: [] })
	}
})
