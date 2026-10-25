// SPDX-License-Identifier: AGPL-3.0-or-later

import { list } from '@alphone/frontend-sdk'
import { HttpResponse, fakeGraphClient, graphql, server } from '@alphone/frontend-sdk/testing'
import { render, screen } from '@testing-library/react'
import { expect, test } from 'vitest'

import { FieldsScreen } from '../FieldsScreen'
import { plugin } from '../index'
import { renderHosted } from './harness'

test('titles an empty catalogue under the page and draws the list glyph its sidebar row draws', async () => {
	const { container, unmount } = render(list)
	const drawing = container.querySelector('svg')?.innerHTML
	unmount()
	server.use(
		graphql.query('FieldCatalogue', () =>
			HttpResponse.json({ data: { fields: [], every: [], reservedFieldNames: [] } }),
		),
	)

	renderHosted(<FieldsScreen />, fakeGraphClient([plugin]).graph)

	const title = await screen.findByRole('heading', { level: 3, name: 'No fields yet.' })
	expect(title.closest('.godmin-empty')?.querySelector('svg')?.innerHTML).toBe(drawing)
	expect(plugin.nav[0].icon).toBe(list)
})
