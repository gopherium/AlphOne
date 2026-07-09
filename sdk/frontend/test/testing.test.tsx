// SPDX-License-Identifier: AGPL-3.0-or-later

import { expect, test } from 'vitest'

import { FakeEventSource } from '../testing'

test('last() explains itself when no EventSource was created', () => {
	FakeEventSource.reset()

	expect(() => FakeEventSource.last()).toThrow('no EventSource was created')
})

test('emit() without a listener is a safe no-op', () => {
	const source = new FakeEventSource('/stream')

	source.emit()

	expect(source.closed).toBe(false)
})
