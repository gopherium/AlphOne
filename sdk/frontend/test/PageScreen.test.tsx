// SPDX-License-Identifier: AGPL-3.0-or-later

import { render, screen } from '@testing-library/react'
import { expect, test, vi } from 'vitest'

import { LoadMore, PageScreen, ValidationError, validationMessage } from '../index'

test('renders the title as the page heading', () => {
	render(<PageScreen title="Contacts">body</PageScreen>)

	expect(screen.getByRole('heading', { level: 1, name: 'Contacts' })).toBeInTheDocument()
	expect(screen.getByText('body')).toBeInTheDocument()
})

test('renders the subtitle under the title', () => {
	render(
		<PageScreen title="Tasks" subtitle="Friday, Aug 1">
			body
		</PageScreen>,
	)

	expect(screen.getByText('Friday, Aug 1')).toBeInTheDocument()
})

test('renders the actions beside the title', () => {
	render(
		<PageScreen title="Tasks" actions={<button type="button">New task</button>}>
			body
		</PageScreen>,
	)

	expect(screen.getByRole('button', { name: 'New task' })).toBeInTheDocument()
})

test('passes a class through to the page wrapper', () => {
	const { container } = render(
		<PageScreen title="Tasks" className="alphone-tasks">
			body
		</PageScreen>,
	)

	expect(container.querySelector('.alphone-tasks')).not.toBeNull()
})

test('load more renders nothing without a next page', () => {
	render(
		<LoadMore query={{ hasNextPage: false, isFetchingNextPage: false, fetchNextPage: vi.fn() }}>
			Load more
		</LoadMore>,
	)

	expect(screen.queryByRole('button', { name: 'Load more' })).not.toBeInTheDocument()
})

test('load more fetches the next page on click', () => {
	const fetchNextPage = vi.fn().mockResolvedValue(undefined)
	render(
		<LoadMore query={{ hasNextPage: true, isFetchingNextPage: false, fetchNextPage }}>
			Load more done
		</LoadMore>,
	)

	screen.getByRole('button', { name: 'Load more done' }).click()

	expect(fetchNextPage).toHaveBeenCalledOnce()
})

test('load more disables while a page is in flight', () => {
	render(
		<LoadMore query={{ hasNextPage: true, isFetchingNextPage: true, fetchNextPage: vi.fn() }}>
			Load more
		</LoadMore>,
	)

	expect(screen.getByRole('button', { name: 'Load more' })).toHaveAttribute('aria-disabled', 'true')
})

test('validation messages surface verbatim and anything else falls back', () => {
	expect(validationMessage(new ValidationError('task: empty title'), 'fallback')).toBe(
		'task: empty title',
	)
	expect(validationMessage(new Error('boom'), 'fallback')).toBe('fallback')
})
