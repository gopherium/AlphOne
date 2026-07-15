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

test('exposes the WHATWG readyState constants', () => {
	expect(FakeEventSource.CONNECTING).toBe(0)
	expect(FakeEventSource.OPEN).toBe(1)
	expect(FakeEventSource.CLOSED).toBe(2)
})

test('a new source starts in the CONNECTING state', () => {
	const source = new FakeEventSource('/stream')

	expect(source.readyState).toBe(FakeEventSource.CONNECTING)
})

test('emitOpen() reports the OPEN state to the open listener', () => {
	const source = new FakeEventSource('/stream')
	const states: number[] = []
	source.onopen = () => states.push(source.readyState)

	source.emitOpen()

	expect(states).toEqual([FakeEventSource.OPEN])
})

test('emitError() reports the given state to the error listener', () => {
	const source = new FakeEventSource('/stream')
	const states: number[] = []
	source.onerror = () => states.push(source.readyState)

	source.emitError(FakeEventSource.CONNECTING)
	source.emitError(FakeEventSource.CLOSED)

	expect(states).toEqual([FakeEventSource.CONNECTING, FakeEventSource.CLOSED])
})

test('close() moves the source to the CLOSED state', () => {
	const source = new FakeEventSource('/stream')

	source.close()

	expect(source.readyState).toBe(FakeEventSource.CLOSED)
})

test('emitOpen() and emitError() without listeners are safe no-ops', () => {
	const source = new FakeEventSource('/stream')

	source.emitOpen()
	source.emitError(FakeEventSource.CLOSED)

	expect(source.readyState).toBe(FakeEventSource.CLOSED)
})
