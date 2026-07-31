// SPDX-License-Identifier: AGPL-3.0-or-later

import { assertElementPatched } from '@gopherium/godmin/testing'
import { expect, test } from 'vitest'

test('@wordpress/element works on React 19', async () => {
	await expect(assertElementPatched()).resolves.toBeUndefined()
})

test('components can ask whether a media query matches', () => {
	expect(typeof window.matchMedia).toBe('function')
	expect(window.matchMedia('(min-width: 600px)').media).toBe('(min-width: 600px)')
})
