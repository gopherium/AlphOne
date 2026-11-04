// SPDX-License-Identifier: AGPL-3.0-or-later

import { renderHook } from '@testing-library/react'
import { expect, test } from 'vitest'

import { useLastAnswer } from '../lastAnswer'

/** first is the answer the read gives first. */
const first = { name: 'first answer' }

/** second is the answer a later read gives. */
const second = { name: 'second answer' }

/** HeldRead is what the read holds at one render. */
interface HeldRead {
	data: typeof first | null | undefined
}

/**
 * Renders the hook over a read holding nothing yet.
 * @returns What the test reads and how it moves the read on.
 */
function renderRead() {
	const nothing: HeldRead = { data: undefined }
	return renderHook(({ data }: HeldRead) => useLastAnswer(data), { initialProps: nothing })
}

test('holds nothing until the first answer', () => {
	expect(renderRead().result.current).toBeUndefined()
})

test('holds nothing while the first read has failed', () => {
	const { result, rerender } = renderRead()

	rerender({ data: null })

	expect(result.current).toBeUndefined()
})

test('keeps the answer through a later read that holds nothing', () => {
	const { result, rerender } = renderRead()

	rerender({ data: first })
	rerender({ data: null })

	expect(result.current).toBe(first)
})

test('keeps an answer that reads as false', () => {
	const { result, rerender } = renderHook(({ count }: { count: number | undefined }) => useLastAnswer(count), {
		initialProps: { count: 0 as number | undefined },
	})

	rerender({ count: undefined })

	expect(result.current).toBe(0)
})

test('moves on to the next answer', () => {
	const { result, rerender } = renderRead()

	rerender({ data: first })
	rerender({ data: null })
	rerender({ data: second })

	expect(result.current).toBe(second)
})
