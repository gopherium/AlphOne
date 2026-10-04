// SPDX-License-Identifier: AGPL-3.0-or-later

import { act, renderHook } from '@testing-library/react'
import { expect, test } from 'vitest'

import { pageWindow, paginationOf, useServerPaging } from '../index'
import type { PagedView, PageWindow, PaginationInfo, ServedPage, ServedSize, ServerPaging } from '../index'

test('hands plugins the rows a server paged view asks for, stepping by the size served', () => {
	const view: PagedView = { page: 3, perPage: 20 }
	const last: ServedSize = { asked: 20, served: 10 }

	const window: PageWindow = pageWindow(view, last, 50)

	expect(window).toEqual({ limit: 20, offset: 20 })
})

test('hands plugins the page count a served page fills', () => {
	const page: ServedPage = { total: 45, limit: 20 }

	const info: PaginationInfo = paginationOf(page)

	expect(info).toEqual({ totalItems: 45, totalPages: 3 })
})

test('hands plugins the hook recording the size the server served', () => {
	const view: PagedView = { page: 2, perPage: 20 }
	const { result } = renderHook(() => useServerPaging(view, 200))

	act(() => result.current.record({ total: 45, limit: 10 }, 20))
	const paging: ServerPaging = result.current

	expect(paging.window).toEqual({ limit: 20, offset: 10 })
})
