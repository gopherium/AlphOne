// SPDX-License-Identifier: AGPL-3.0-or-later

import { readdirSync, readFileSync, statSync } from 'node:fs'
import { join } from 'node:path'

import { expect, test } from '@playwright/test'

// assets is the built output the e2e server serves.
const assets = join(process.cwd(), '..', '..', 'frontend', 'dist', 'assets')

// entryCeiling bounds the chunk every visitor downloads, in bytes.
const entryCeiling = 700 * 1024

// previewCeiling bounds the lazy chunk only the import preview downloads.
const previewCeiling = 1900 * 1024

/**
 * Returns the built asset whose name starts with the given prefix.
 * @param prefix - The chunk name before its hash.
 * @param extension - The file extension to match.
 * @returns The file name.
 */
function chunk(prefix: string, extension: string): string {
	const found = readdirSync(assets).filter(
		(name) => name.startsWith(prefix) && name.endsWith(extension),
	)
	expect(found, `one ${prefix}*${extension} chunk`).toHaveLength(1)
	return found[0]
}

test('the entry bundle stays free of the import preview', () => {
	const entry = chunk('index-', '.js')

	const source = readFileSync(join(assets, entry), 'utf8')

	expect(source).not.toContain('DataViews')
	expect(statSync(join(assets, entry)).size).toBeLessThan(entryCeiling)
})

test('the import preview stays behind its own chunk', () => {
	const preview = chunk('RowsTable-', '.js')

	expect(statSync(join(assets, preview)).size).toBeLessThan(previewCeiling)
	expect(readFileSync(join(assets, preview), 'utf8')).toContain('DataViews')
})
