// SPDX-License-Identifier: AGPL-3.0-or-later

import { render, screen, within } from '@testing-library/react'
import { expect, test } from 'vitest'

import { Card, Text } from '../index'

test('hands plugins a card whose header names the content it frames', () => {
	render(
		<Card.Root role="group" aria-labelledby="visits">
			<Card.Header>
				<Card.Title id="visits" render={<h3 />}>
					Visits
				</Card.Title>
			</Card.Header>
			<Card.Content>
				<Text>No entries yet.</Text>
			</Card.Content>
		</Card.Root>,
	)

	const card = screen.getByRole('group', { name: 'Visits' })
	const [header, content] = [...card.children]
	expect(header).toContainElement(within(card).getByRole('heading', { level: 3, name: 'Visits' }))
	expect(content).toHaveTextContent('No entries yet.')
})
