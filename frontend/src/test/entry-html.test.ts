// SPDX-License-Identifier: AGPL-3.0-or-later

import { expect, test } from 'vitest'

const sources = import.meta.glob('../../index.html', {
	query: '?raw',
	import: 'default',
	eager: true,
}) as Record<string, string>

const entry = sources['../../index.html']

test('stands a ghost in the root before any script runs', () => {
	const root = entry.slice(entry.indexOf('<div id="root"'))

	expect(root).toMatch(/alphone-boot__bar/)
	expect(root).toMatch(/aria-hidden="true"/)
})

test('holds that ghost back so a quick load never shows it', () => {
	expect(entry).toMatch(/\.alphone-boot\s*\{[^}]*opacity:\s*0/)
	expect(entry).toMatch(/\.alphone-boot\s*\{[^}]*animation:[^;]*150ms/)
	expect(entry).toMatch(/@keyframes alphone-boot-in/)
})
