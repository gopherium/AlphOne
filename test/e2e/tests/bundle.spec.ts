// SPDX-License-Identifier: AGPL-3.0-or-later

import { readdirSync, readFileSync, statSync } from 'node:fs'
import { join } from 'node:path'

import { expect, test } from '@playwright/test'

// dist is the built output the e2e server serves.
const dist = join(process.cwd(), '..', '..', 'frontend', 'dist')

// assets is the hashed chunk directory inside the build.
const assets = join(dist, 'assets')

// entryCeiling bounds the JavaScript every visitor downloads before first paint, in bytes.
const entryCeiling = 1240 * 1024

// listCeiling bounds the lazy chunk every DataViews screen shares.
const listCeiling = 1900 * 1024

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

/**
 * Returns every built script that carries DataViews.
 * @returns The asset file names.
 */
function dataViewsChunks(): string[] {
	return readdirSync(assets)
		.filter((name) => name.endsWith('.js'))
		.filter((name) => readFileSync(join(assets, name), 'utf8').includes('DataViews'))
}

test('nothing loaded before first paint carries DataViews', () => {
	for (const name of eagerScripts()) {
		expect(readFileSync(join(assets, name), 'utf8'), name).not.toContain('DataViews')
	}
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

test('DataViews lives in one lazy chunk under its ceiling', () => {
	const found = dataViewsChunks()

	expect(found, 'one chunk carries DataViews').toHaveLength(1)
	expect(eagerScripts()).not.toContain(found[0])
	expect(statSync(join(assets, found[0])).size).toBeLessThan(listCeiling)
})

test('the users list, the tokens list, the imports list and the import preview each load DataViews on demand', () => {
	const [shared] = dataViewsChunks()

	for (const screen of ['UsersScreen-', 'TokensScreen-', 'ImportsScreen-', 'RowsTable-']) {
		const lazy = chunk(screen, '.js')
		expect(eagerScripts()).not.toContain(lazy)
		expect(readFileSync(join(assets, lazy), 'utf8'), lazy).toContain(shared)
	}
})

test('the contacts list loads DataViews on demand', () => {
	const [shared] = dataViewsChunks()
	const lazy = chunk('ContactsScreen-', '.js')

	expect(eagerScripts()).not.toContain(lazy)
	expect(readFileSync(join(assets, lazy), 'utf8'), lazy).toContain(shared)
})
