// SPDX-License-Identifier: AGPL-3.0-or-later

import { http, HttpResponse, server } from '@alphone/frontend-sdk/testing'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import {
	RouterProvider,
	createMemoryHistory,
	createRootRoute,
	createRoute,
	createRouter,
} from '@tanstack/react-router'
import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { beforeEach, expect, test } from 'vitest'

import { handlers, importID } from '../handlers'
import { uploadChosen } from '../ImportsScreen'
import { routes } from '../routes'

/**
 * Renders the importer routes at the given path.
 * @param path - The path to start at.
 */
function renderAt(path: string) {
	const rootRoute = createRootRoute()
	const indexRoute = createRoute({ getParentRoute: () => rootRoute, path: '/' })
	const router = createRouter({
		routeTree: rootRoute.addChildren([indexRoute, ...routes(rootRoute)]),
		history: createMemoryHistory({ initialEntries: [path] }),
	})
	const client = new QueryClient({ defaultOptions: { queries: { retry: false } } })
	render(
		<QueryClientProvider client={client}>
			<RouterProvider router={router as never} />
		</QueryClientProvider>,
	)
}

beforeEach(() => {
	server.use(...handlers)
})

test('the history ghosts its rows while they arrive', async () => {
	server.use(http.get('/api/plugins/importer/imports', () => new Promise(() => {})))
	renderAt('/import')

	const status = await screen.findByRole('status')
	expect(status).toHaveTextContent('Loading imports…')
	expect(status.closest('.godmin-loading-rows')).not.toBeNull()
})

test('the history lists every stored import with its counts', async () => {
	renderAt('/import')

	expect(await screen.findByRole('link', { name: 'contacts.csv' })).toBeInTheDocument()
	const row = screen.getByRole('row', { name: /contacts\.csv/ })
	expect(row).toHaveTextContent('ready')
})

test('the history shows an empty state without imports', async () => {
	server.use(http.get('/api/plugins/importer/imports', () => HttpResponse.json([])))

	renderAt('/import')

	expect(await screen.findByText('No imports yet.')).toBeInTheDocument()
})

test('the history reports a failed read', async () => {
	server.use(
		http.get('/api/plugins/importer/imports', () =>
			HttpResponse.json({ error: 'nope' }, { status: 500 }),
		),
	)

	renderAt('/import')

	expect(await screen.findByRole('alert')).toHaveTextContent('Imports could not be loaded.')
})

test('choosing a file uploads it and refreshes the history', async () => {
	let uploads = 0
	server.use(
		http.post('/api/plugins/importer/imports', () => {
			uploads++
			return HttpResponse.json(
				{
					id: importID,
					user_id: '019f5a00-0000-7000-8000-0000000000aa',
					filename: 'uploaded.csv',
					state: 'ready',
					row_count: 1,
					imported_count: 0,
					skipped_count: 0,
					failed_count: 0,
					created_at: '2026-08-01T10:00:00Z',
					columns: ['Name', 'Email'],
					mapping: {},
				},
				{ status: 201 },
			)
		}),
		http.get('/api/plugins/importer/imports', () =>
			HttpResponse.json(
				uploads === 0
					? []
					: [
							{
								id: importID,
								user_id: '019f5a00-0000-7000-8000-0000000000aa',
								filename: 'uploaded.csv',
								state: 'ready',
								row_count: 1,
								imported_count: 0,
								skipped_count: 0,
								failed_count: 0,
								created_at: '2026-08-01T10:00:00Z',
							},
						],
			),
		),
	)
	renderAt('/import')
	await screen.findByText('No imports yet.')

	const file = new File(['Name,Email\nMaria Perez,maria@example.com\n'], 'uploaded.csv', {
		type: 'text/csv',
	})
	await userEvent.upload(screen.getByLabelText('Contacts file'), file)

	expect(await screen.findByRole('link', { name: 'uploaded.csv' })).toBeInTheDocument()
	expect(uploads).toBe(1)
})

test('cancelling the file dialog uploads nothing', () => {
	let uploads = 0
	const count = () => uploads++

	uploadChosen(null, count)
	uploadChosen([] as unknown as FileList, count)

	expect(uploads).toBe(0)
})

test('choosing a file hands it to the upload', () => {
	const chosen: File[] = []
	const file = new File(['Name\n'], 'contacts.csv')

	uploadChosen([file] as unknown as FileList, (picked) => chosen.push(picked))

	expect(chosen).toEqual([file])
})

test('a refused upload is reported', async () => {
	server.use(
		http.post('/api/plugins/importer/imports', () =>
			HttpResponse.json({ error: 'the file is neither a CSV nor an Excel workbook' }, { status: 422 }),
		),
	)
	renderAt('/import')
	await screen.findByRole('link', { name: 'contacts.csv' })

	const file = new File(['nope'], 'notes.csv', { type: 'text/csv' })
	await userEvent.upload(screen.getByLabelText('Contacts file'), file)

	expect(await screen.findByRole('alert')).toHaveTextContent(
		'the file is neither a CSV nor an Excel workbook',
	)
})
