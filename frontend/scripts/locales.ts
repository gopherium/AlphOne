// SPDX-License-Identifier: AGPL-3.0-or-later

import { readFileSync } from 'node:fs'
import { join } from 'node:path'

/** LOCALE_SOURCE is where the server declares the languages it answers in. */
const LOCALE_SOURCE = ['internal', 'locale', 'locale.go']

/**
 * Returns the languages the server answers in, as it declares them.
 * @param root - The repository root the declaration sits under.
 * @returns The languages, the default first.
 */
export function supportedLocales(root: string): string[] {
	const source = readFileSync(join(root, ...LOCALE_SOURCE), 'utf8')
	const declared = /Default\s*=\s*"([^"]+)"/.exec(source)
	const listed = /supported\s*=\s*\[\]string\{([^}]*)\}/.exec(source)
	if (declared === null || listed === null) {
		throw new Error('the server declares no supported languages')
	}
	return listed[1]
		.split(',')
		.map((held) => held.trim())
		.filter((held) => held !== '')
		.map((held) => (held === 'Default' ? declared[1] : held.replace(/"/g, '')))
}
