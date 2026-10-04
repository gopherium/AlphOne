// SPDX-License-Identifier: AGPL-3.0-or-later

import { pot } from '@gopherium/gottext/build'
import { expect, test } from 'vitest'

import { domains, potConfig } from '../../scripts/config.ts'

/** NUMBER_PLACEHOLDER matches a placeholder that writes a bare number instead of formatted text. */
const NUMBER_PLACEHOLDER = /%(?:\(\w+\))?[ +0#-]*\d*(?:\.\d+)?[dfi]/

/** overlaid holds the catalogue template of every enterprise plugin laid over this tree. */
const overlaid = import.meta.glob('../../../enterprise/*/languages/*.pot', {
	query: '?raw',
	import: 'default',
	eager: true,
}) as Record<string, string>

/**
 * Returns every message of one catalogue template that writes a bare number.
 * @param template - The catalogue template.
 * @returns The lines naming such a message.
 */
function bareLines(template: string): string[] {
	return template
		.split('\n')
		.filter((line) => line.startsWith('msgid') || line.startsWith('"'))
		.filter((line) => NUMBER_PLACEHOLDER.test(line))
}

/**
 * Returns every message of one domain that writes a bare number.
 * @param domain - The domain to extract.
 * @returns The lines naming such a message.
 */
function bareNumbers(domain: ReturnType<typeof domains>[number]): string[] {
	return bareLines(pot(potConfig(domain)).toString('utf8'))
}

test('takes every number a message shows as text the format locale wrote', () => {
	for (const domain of domains()) {
		expect({ domain: domain.name, bare: bareNumbers(domain) }).toEqual({ domain: domain.name, bare: [] })
	}
})

test('takes every number an overlaid plugin message shows as text the format locale wrote', () => {
	for (const [path, template] of Object.entries(overlaid)) {
		expect({ path, bare: bareLines(template) }).toEqual({ path, bare: [] })
	}
})
