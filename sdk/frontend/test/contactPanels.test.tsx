// SPDX-License-Identifier: AGPL-3.0-or-later

import { render, screen } from '@testing-library/react'
import { expect, test } from 'vitest'

import type { ContactPanel, FrontendPlugin } from '../index'

test('a plugin may contribute no contact panels', () => {
	const plugin: FrontendPlugin = { id: 'quiet', routes: () => [], nav: [] }

	expect(plugin.contactPanels).toBeUndefined()
})

test('a contributed panel renders for the given contact', () => {
	const panel: ContactPanel = {
		id: 'fields',
		Panel: ({ contactId }) => <p>{`panel for ${contactId}`}</p>,
	}
	const plugin: FrontendPlugin = {
		id: 'fields',
		routes: () => [],
		nav: [],
		contactPanels: [panel],
	}

	const [contributed] = plugin.contactPanels ?? []
	render(<contributed.Panel contactId="0198c000-0000-7000-8000-000000000401" />)

	expect(
		screen.getByText('panel for 0198c000-0000-7000-8000-000000000401'),
	).toBeInTheDocument()
})
