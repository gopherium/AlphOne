// SPDX-License-Identifier: AGPL-3.0-or-later

import { readFileSync } from 'node:fs'
import { join } from 'node:path'

import { declarationsIn } from '@alphone/frontend-sdk/testing'

/** sheet is the stylesheet the application ships. */
const sheet = readFileSync(join(import.meta.dirname, '..', 'index.css'), 'utf8')

/**
 * Returns the declarations of the rule for exactly one selector, at the top level or inside the given media block.
 * @param selector - The selector of the rule.
 * @param media - The condition of the media block holding the rule, such as (max-width: 639px).
 * @returns The declarations, keyed by property.
 */
export function declarations(selector: string, media?: string): Record<string, string> {
	return declarationsIn(sheet, selector, media)
}
