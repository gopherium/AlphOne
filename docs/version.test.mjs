// SPDX-License-Identifier: AGPL-3.0-or-later

import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import { test } from 'node:test'
import { fileURLToPath } from 'node:url'

import { pinsALiteral, remarkVersion, substitute, version } from './version.mjs'

const released = readFileSync(
	fileURLToPath(new URL('../internal/version/VERSION', import.meta.url)),
	'utf8',
).trim()

test('the version derives from the file the binary embeds', () => {
	assert.equal(version, released)
})

test('substitute puts the release into a token', () => {
	assert.equal(substitute('AlphOne %VERSION% or newer'), `AlphOne ${released} or newer`)
})

test('substitute leaves text carrying no token alone', () => {
	assert.equal(substitute('nothing to see'), 'nothing to see')
})

test('a literal of the current release is refused', () => {
	assert.equal(pinsALiteral(`{ "version": "${released}" }`), true)
})

test('a tag spelling of the current release is refused', () => {
	assert.equal(pinsALiteral(`the v${released} tag`), true)
})

test('an older release stays legal, so history reads correctly', () => {
	assert.equal(pinsALiteral('the routes were removed in 0.8.0'), false)
})

test('a longer version merely containing the release is not refused', () => {
	assert.equal(pinsALiteral(`10.${released} and ${released}.1`), false)
})

test('the plugin substitutes tokens through the tree', () => {
	const tree = {
		type: 'root',
		children: [{ type: 'paragraph', children: [{ type: 'text', value: 'AlphOne %VERSION%' }] }],
	}

	remarkVersion()(tree, { path: 'guides/agents.md' })

	assert.equal(tree.children[0].children[0].value, `AlphOne ${released}`)
})

test('the plugin refuses a literal and names the file', () => {
	const tree = { type: 'root', children: [{ type: 'code', value: `"version": "${released}"` }] }

	assert.throws(() => remarkVersion()(tree, { path: 'guides/automation.md' }), {
		message: /guides\/automation\.md.*%VERSION%/s,
	})
})
