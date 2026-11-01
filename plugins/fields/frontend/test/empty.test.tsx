// SPDX-License-Identifier: AGPL-3.0-or-later

import { list } from '@alphone/frontend-sdk'
import { HttpResponse, fakeGraphClient, graphql, server } from '@alphone/frontend-sdk/testing'
import { render, screen } from '@testing-library/react'
import { expect, test } from 'vitest'

import { FieldsScreen } from '../FieldsScreen'
import { plugin } from '../index'
import { renderHosted } from './harness'

test('titles an empty catalogue under the page, describes it and draws the list glyph of its sidebar row', async () => {
	const { container, unmount } = render(list)
	const drawing = container.querySelector('svg')?.innerHTML
	unmount()
	server.use(
		graphql.query('FieldCatalogue', () =>
			HttpResponse.json({ data: { fields: [], every: [], reservedFieldNames: [] } }),
		),
	)

	renderHosted(<FieldsScreen />, fakeGraphClient([plugin]).graph)

	const title = await screen.findByRole('heading', { level: 3, name: 'No fields yet' })
	const empty = title.closest('.godmin-empty')
	expect(empty?.querySelector('svg')?.innerHTML).toBe(drawing)
	expect(empty).toHaveTextContent('Add a field to store more about every contact.')
	expect(plugin.nav[0].icon).toBe(list)
})
