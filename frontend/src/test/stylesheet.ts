// SPDX-License-Identifier: AGPL-3.0-or-later

import { readFileSync } from 'node:fs'
import { join } from 'node:path'

/** sheet is the stylesheet the application ships. */
const sheet = readFileSync(join(import.meta.dirname, '..', 'index.css'), 'utf8')

/**
 * Returns the declarations of the top level rule written for exactly one selector.
 * @param selector - The selector of the rule.
 * @returns The declarations, keyed by property.
 */
export function declarations(selector: string): Record<string, string> {
	const escaped = selector.replace(/[.*+?^${}()|[\]\\]/g, '\\$&')
	const body = new RegExp(`(?:^|\\n)${escaped} \\{([^}]*)\\}`).exec(sheet)?.[1] ?? ''
	return Object.fromEntries(
		body
			.split(';')
			.map((declaration) => declaration.trim())
			.filter((declaration) => declaration !== '')
			.map((declaration) => {
				const at = declaration.indexOf(':')
				return [declaration.slice(0, at).trim(), declaration.slice(at + 1).trim()]
			}),
	)
}
