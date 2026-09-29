// SPDX-License-Identifier: AGPL-3.0-or-later

import { render, screen } from '@testing-library/react'
import { expect, test, vi } from 'vitest'

import {
	ErrorNotice,
	IconButton,
	LoadMore,
	PageScreen,
	SectionTitle,
	ValidationError,
	pencil,
	trash,
	validationMessage,
} from '../index'
import { textClasses } from '../testing'

test('hands plugins a section title a size above the field labels', () => {
	render(<SectionTitle>Fields</SectionTitle>)

	const heading = screen.getByRole('heading', { level: 2, name: 'Fields' })
	expect([...heading.classList]).toEqual(expect.arrayContaining(textClasses('heading-lg')))
	expect([...heading.classList].sort()).not.toEqual(textClasses('heading-sm').sort())
})

test('hands plugins the pencil and trash icons', () => {
	render(
		<>
			<IconButton icon={pencil} label="Edit entry: Sep 1, 2026, First call." />
			<IconButton icon={trash} label="Remove entry: Sep 1, 2026, First call." />
		</>,
	)

	expect(screen.getByRole('button', { name: 'Edit entry: Sep 1, 2026, First call.' })).toBeInTheDocument()
	expect(screen.getByRole('button', { name: 'Remove entry: Sep 1, 2026, First call.' })).toBeInTheDocument()
})

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

test('every page carries the shared page class that fixes its width', () => {
	const { container: plain } = render(<PageScreen title="Users">body</PageScreen>)
	const { container: classed } = render(
		<PageScreen title="Tasks" className="alphone-tasks">
			body
		</PageScreen>,
	)

	expect(plain.querySelector('.godmin-page')).not.toBeNull()
	expect(classed.querySelector('.godmin-page.alphone-tasks')).not.toBeNull()
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

test('error notices announce themselves to a screen reader', () => {
	render(<ErrorNotice>Contacts could not be loaded.</ErrorNotice>)

	expect(screen.getByRole('alert')).toHaveTextContent('Contacts could not be loaded.')
})

test('validation messages surface verbatim and anything else falls back', () => {
	expect(validationMessage(new ValidationError('task: empty title'), 'fallback')).toBe(
		'task: empty title',
	)
	expect(validationMessage(new Error('boom'), 'fallback')).toBe('fallback')
})
