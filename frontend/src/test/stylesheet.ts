// SPDX-License-Identifier: AGPL-3.0-or-later

import { readFileSync } from 'node:fs'
import { join } from 'node:path'

/** sheet is the stylesheet the application ships. */
const sheet = readFileSync(join(import.meta.dirname, '..', 'index.css'), 'utf8')

/**
 * Escapes a string for use inside a regular expression.
 * @param text - The literal text.
 * @returns The escaped text.
 */
function literal(text: string): string {
	return text.replace(/[.*+?^${}()|[\]\\]/g, '\\$&')
}

/**
 * Returns the declarations of the rule for exactly one selector, at the top level or inside the given media block.
 * @param selector - The selector of the rule.
 * @param media - The condition of the media block holding the rule, such as (max-width: 639px).
 * @returns The declarations, keyed by property.
 */
export function declarations(selector: string, media?: string): Record<string, string> {
	const block =
		media === undefined ? sheet : (new RegExp(`\\n@media ${literal(media)} \\{([\\s\\S]*?)\\n\\}`).exec(sheet)?.[1] ?? '')
	const indent = media === undefined ? '' : '\\t'
	const body = new RegExp(`(?:^|\\n)${indent}${literal(selector)} \\{([^}]*)\\}`).exec(block)?.[1] ?? ''
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
