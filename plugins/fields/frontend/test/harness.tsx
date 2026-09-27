// SPDX-License-Identifier: AGPL-3.0-or-later

import { GraphProvider } from '@alphone/frontend-sdk'
import { HttpResponse, fakeGraphClient, graphql, server } from '@alphone/frontend-sdk/testing'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { render } from '@testing-library/react'
import { vi } from 'vitest'

import { ContactFieldsPanel } from '../ContactFieldsPanel'

/** contactID is the contact every panel test renders. */
export const contactID = '0198c000-0000-7000-8000-000000000401'

/** jobTitle is a plain text field the panel tests define. */
export const jobTitle = {
	__typename: 'FieldDefinition',
	id: '0198c000-0000-7000-8000-000000000504',
	name: 'jobTitle',
	label: 'Job title',
	kind: 'TEXT',
	subFields: [],
}

/** history is a repeater of a date and a comment the panel tests define. */
export const history = {
	__typename: 'FieldDefinition',
	id: '0198c000-0000-7000-8000-000000000506',
	name: 'history',
	label: 'History',
	kind: 'REPEATER',
	subFields: [
		{ __typename: 'FieldSubField', name: 'date', label: 'Date', kind: 'DATE' },
		{ __typename: 'FieldSubField', name: 'comment', label: 'Comment', kind: 'LONGTEXT' },
	],
}

/** firstCall is the older stored history entry. */
export const firstCall = { date: '2026-09-01', comment: 'First call about the yearly plan.' }

/** offerSent is the newer stored history entry. */
export const offerSent = { date: '2026-09-10', comment: 'Sent the offer and booked a follow-up call.' }

/**
 * Renders the fields panel of the test contact inside its graph and query providers.
 * @returns The render result.
 */
export function renderPanel() {
	const client = new QueryClient({ defaultOptions: { queries: { retry: false } } })
	const { graph } = fakeGraphClient()
	return render(
		<QueryClientProvider client={client}>
			<GraphProvider graph={graph}>
				<ContactFieldsPanel contactId={contactID} />
			</GraphProvider>
		</QueryClientProvider>,
	)
}

/**
 * Answers the catalogue query with the given fields.
 * @param fields - The definitions the catalogue holds.
 */
export function serveCatalogue(fields: unknown[]) {
	server.use(graphql.query('Fields', () => HttpResponse.json({ data: { fields } })))
}

/**
 * Answers the values query of the test contact with the given values.
 * @param values - The stored values, keyed by field name.
 */
export function serveValues(values: Record<string, unknown>) {
	server.use(
		graphql.query('ContactFieldValues', () =>
			HttpResponse.json({
				data: { contact: { __typename: 'Contact', id: contactID, ...values } },
			}),
		),
	)
}

/**
 * Accepts every field write, recording the variables each one carries.
 * @returns The mock the variables are recorded on.
 */
export function captureWrite() {
	const written = vi.fn()
	server.use(
		graphql.mutation('WriteContactFields', async ({ variables }) => {
			written(variables)
			return HttpResponse.json({ data: { writeContactFields: true } })
		}),
	)
	return written
}
