// SPDX-License-Identifier: AGPL-3.0-or-later

/// <reference types="node" />
import { readFileSync } from 'node:fs'
import { join } from 'node:path'

import { pinnedVersions, resolvedVersions } from '@gopherium/gottext/build'
import { expect, test } from 'vitest'

/** repositoryRoot returns the directory the workspaces sit under. */
function repositoryRoot(): string {
	return join(import.meta.dirname, '..', '..', '..')
}

/** PINNING lists every workspace declaring the translation packages. */
const PINNING = ['frontend', 'sdk/frontend']

/** RUNTIME is the package holding the module scoped catalogue. */
const RUNTIME = '@wordpress/i18n'

/** BRICK is the package holding the shared translation seams. */
const BRICK = '@gopherium/gottext'

/** lockfile reads the workspace lockfile the resolution gate walks. */
function lockfile(): string {
	return readFileSync(join(repositoryRoot(), 'pnpm-lock.yaml'), 'utf8')
}

test('resolves exactly one copy of the translation runtime', () => {
	expect(resolvedVersions(lockfile(), RUNTIME)).toHaveLength(1)
})

test('resolves exactly one copy of the translation brick', () => {
	expect(resolvedVersions(lockfile(), BRICK)).toHaveLength(1)
})

test('pins the runtime exactly, at the one resolved version', () => {
	const resolved = resolvedVersions(lockfile(), RUNTIME)

	for (const pinned of pinnedVersions(repositoryRoot(), PINNING, RUNTIME)) {
		expect(pinned).toBe(resolved[0])
	}
})

test('pins the brick exactly, at the one resolved version', () => {
	const resolved = resolvedVersions(lockfile(), BRICK)

	for (const pinned of pinnedVersions(repositoryRoot(), PINNING, BRICK)) {
		expect(pinned).toBe(resolved[0])
	}
})

test('pins both packages in every workspace that calls them', () => {
	expect(pinnedVersions(repositoryRoot(), PINNING, RUNTIME)).toHaveLength(2)
	expect(pinnedVersions(repositoryRoot(), PINNING, BRICK)).toHaveLength(2)
})

test('reports no pin for a package nothing declares', () => {
	expect(pinnedVersions(repositoryRoot(), PINNING, '@alphone/not-a-package')).toEqual([])
})
