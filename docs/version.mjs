// SPDX-License-Identifier: AGPL-3.0-or-later

import { readFileSync } from 'node:fs'
import { fileURLToPath } from 'node:url'

const versionFile = fileURLToPath(new URL('../internal/version/VERSION', import.meta.url))

/** The release the docs describe. */
export const version = readFileSync(versionFile, 'utf8').trim()

const token = /%VERSION%/g
const escaped = version.replace(/\./g, '\\.')
const literal = new RegExp(`(?<![\\w.])v?${escaped}(?![\\w.])`)
const valued = ['text', 'code', 'inlineCode', 'yaml', 'html']

/**
 * Returns the text with every version token replaced.
 * @param text - The source text to substitute.
 * @returns The text carrying the release version.
 */
export function substitute(text) {
	return text.replace(token, version)
}

/**
 * Reports whether the text pins the release version a bump would leave stale.
 * @param text - The source text to inspect.
 * @returns True when a literal is present.
 */
export function pinsALiteral(text) {
	return literal.test(text)
}

/**
 * Applies the substitution to every valued node beneath the given node.
 * @param node - The tree node to walk.
 * @param file - The file being compiled, named in a refusal.
 */
function walk(node, file) {
	if (valued.includes(node.type) && typeof node.value === 'string') {
		if (pinsALiteral(node.value)) {
			throw new Error(
				`${file.path}: pins the release version, use %VERSION%\n  ${node.value.slice(0, 120)}`,
			)
		}
		node.value = substitute(node.value)
	}
	for (const child of node.children ?? []) {
		walk(child, file)
	}
}

/**
 * Returns a remark plugin substituting version tokens and refusing literals.
 * @returns The remark plugin.
 */
export function remarkVersion() {
	return (tree, file) => walk(tree, file)
}
