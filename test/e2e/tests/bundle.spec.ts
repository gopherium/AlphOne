// SPDX-License-Identifier: AGPL-3.0-or-later

import { readdirSync, readFileSync, statSync } from 'node:fs'
import { join } from 'node:path'

import { expect, test } from '@playwright/test'

// dist is the built output the e2e server serves.
const dist = join(process.cwd(), '..', '..', 'frontend', 'dist')

// assets is the hashed chunk directory inside the build.
const assets = join(dist, 'assets')

// entryCeiling bounds the JavaScript every visitor downloads before first paint, in bytes.
const entryCeiling = 1120 * 1024

// previewCeiling bounds the lazy chunk only the import preview downloads.
const previewCeiling = 1700 * 1024

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

/**
 * Returns every script the entry document loads before first paint.
 * @returns The asset file names, the entry module beside each preloaded chunk.
 */
function eagerScripts(): string[] {
	const html = readFileSync(join(dist, 'index.html'), 'utf8')
	const referenced = [...html.matchAll(/(?:src|href)="\/assets\/([^"]+\.js)"/g)].map(
		(match) => match[1],
	)
	expect(referenced.length, 'the entry document references scripts').toBeGreaterThan(0)
	return referenced
}

test('the entry bundle stays free of the import preview', () => {
	const entry = chunk('index-', '.js')

	const source = readFileSync(join(assets, entry), 'utf8')

	expect(source).not.toContain('DataViews')
})

test('everything loaded before first paint stays under the entry ceiling', () => {
	const scripts = eagerScripts()

	const total = scripts.reduce((sum, name) => sum + statSync(join(assets, name)).size, 0)

	expect(total, `eager payload ${total} bytes across ${scripts.join(', ')}`).toBeLessThan(
		entryCeiling,
	)
})

test('the entry bundle stays free of the test mocks', () => {
	const source = readFileSync(join(assets, chunk('index-', '.js')), 'utf8')

	expect(source).not.toContain('[MSW]')
})

test('the import preview stays behind its own chunk', () => {
	const preview = chunk('RowsTable-', '.js')

	expect(statSync(join(assets, preview)).size).toBeLessThan(previewCeiling)
	expect(readFileSync(join(assets, preview), 'utf8')).toContain('DataViews')
})
