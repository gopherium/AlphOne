// SPDX-License-Identifier: AGPL-3.0-or-later

import { fireEvent, render, screen, within } from '@testing-library/react'
import { CollapsibleCard as wordpressCollapsibleCard } from '@wordpress/ui'
import { expect, test } from 'vitest'

import { Card, CollapsibleCard, Text } from '../index'

test('hands plugins the WordPress collapsible card', () => {
	expect(CollapsibleCard).toBe(wordpressCollapsibleCard)
})

test('hands plugins a collapsible card that folds a message under a header naming its sender', () => {
	render(
		<>
			<CollapsibleCard.Root>
				<CollapsibleCard.Header render={<h3 />}>
					<Card.Title>Maria Perez</Card.Title>
				</CollapsibleCard.Header>
				<CollapsibleCard.Content>
					<Text>Can we meet on Monday?</Text>
				</CollapsibleCard.Content>
			</CollapsibleCard.Root>
			<CollapsibleCard.Root defaultOpen>
				<CollapsibleCard.Header render={<h3 />}>
					<Card.Title>Ada Lovelace</Card.Title>
				</CollapsibleCard.Header>
				<CollapsibleCard.Content>
					<Text>Monday works for me.</Text>
				</CollapsibleCard.Content>
			</CollapsibleCard.Root>
		</>,
	)

	const folded = within(screen.getByRole('heading', { level: 3, name: 'Maria Perez' })).getByRole('button')
	const open = within(screen.getByRole('heading', { level: 3, name: 'Ada Lovelace' })).getByRole('button')
	expect(folded).toHaveAttribute('aria-expanded', 'false')
	expect(screen.getByText('Can we meet on Monday?')).not.toBeVisible()
	expect(open).toHaveAttribute('aria-expanded', 'true')
	expect(screen.getByText('Monday works for me.')).toBeVisible()

	fireEvent.click(folded)

	expect(folded).toHaveAttribute('aria-expanded', 'true')
	expect(screen.getByText('Can we meet on Monday?')).toBeVisible()
})
