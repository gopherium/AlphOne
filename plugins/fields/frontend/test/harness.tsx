// SPDX-License-Identifier: AGPL-3.0-or-later

import { GraphProvider, configureErrorText } from '@alphone/frontend-sdk'
import { HttpResponse, fakeGraphClient, graphql, server } from '@alphone/frontend-sdk/testing'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { render, within } from '@testing-library/react'
import { vi } from 'vitest'

import { ContactFieldsPanel } from '../ContactFieldsPanel'
import { errorTemplates } from '../errorTemplates'

/** contactID is the contact every panel test renders. */
export const contactID = '0198c000-0000-7000-8000-000000000401'

/** otherContactID is a second contact a panel test switches to. */
export const otherContactID = '0198c000-0000-7000-8000-000000000402'

/** ID1 is the id of the oldest stored entry. */
export const ID1 = '0199a3c4-0000-7000-8000-000000000001'

/** ID2 is the id of the middle stored entry. */
export const ID2 = '0199a3c4-0000-7000-8000-000000000002'

/** ID3 is the id of the newest stored entry. */
export const ID3 = '0199a3c4-0000-7000-8000-000000000003'

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

/** visits is a repeater holding one sub field of every kind a row shows differently. */
export const visits = {
	__typename: 'FieldDefinition',
	id: '0198c000-0000-7000-8000-000000000507',
	name: 'visits',
	label: 'Visits',
	kind: 'REPEATER',
	subFields: [
		{ __typename: 'FieldSubField', name: 'date', label: 'Date', kind: 'DATE' },
		{ __typename: 'FieldSubField', name: 'note', label: 'Note', kind: 'TEXT' },
		{ __typename: 'FieldSubField', name: 'minutes', label: 'Minutes', kind: 'NUMBER' },
		{ __typename: 'FieldSubField', name: 'paid', label: 'Paid', kind: 'BOOLEAN' },
		{ __typename: 'FieldSubField', name: 'channel', label: 'Channel', kind: 'SELECT' },
		{ __typename: 'FieldSubField', name: 'followUpOn', label: 'Follow up on', kind: 'DATE' },
	],
}

/** firstCall is the oldest stored history entry. */
export const firstCall = { id: ID1, date: '2026-09-01', comment: 'First call about the yearly plan.' }

/** offerSent is the middle stored history entry. */
export const offerSent = { id: ID2, date: '2026-09-10', comment: 'Sent the offer and booked a follow-up call.' }

/** followUp is the newest stored history entry, its comment on two lines. */
export const followUp = { id: ID3, date: '2026-09-18', comment: 'Follow-up call.\nAsked for a second quote.' }

/**
 * Renders the fields panel of the test contact inside its graph and query providers.
 * @returns The render result, the graph client and a switch to another contact.
 */
export function renderPanel() {
	const client = new QueryClient({ defaultOptions: { queries: { retry: false } } })
	const { graph } = fakeGraphClient()
	const shown = (id: string) => (
		<QueryClientProvider client={client}>
			<GraphProvider graph={graph}>
				<ContactFieldsPanel contactId={id} />
			</GraphProvider>
		</QueryClientProvider>
	)
	const rendered = render(shown(contactID))
	return { ...rendered, graph, showContact: (id: string) => rendered.rerender(shown(id)) }
}

/**
 * Answers every call of the named mutation with the given body, recording the variables each one carries.
 * @param operation - The mutation name.
 * @param body - The body every call answers.
 * @returns The mock the variables are recorded on.
 */
export function capture(operation: string, body: Record<string, unknown>) {
	const called = vi.fn()
	server.use(
		graphql.mutation(operation, ({ variables }) => {
			called(variables)
			return HttpResponse.json(body)
		}),
	)
	return called
}

/**
 * Holds every call of the named mutation until the test releases it, then answers with the given body.
 * @param operation - The mutation name.
 * @param body - The body the released calls answer.
 * @returns The mock the variables are recorded on and the release.
 */
export function hold(operation: string, body: Record<string, unknown>) {
	const called = vi.fn()
	let release = () => {}
	const released = new Promise<void>((resolve) => {
		release = resolve
	})
	server.use(
		graphql.mutation(operation, async ({ variables }) => {
			called(variables)
			await released
			return HttpResponse.json(body)
		}),
	)
	return { called, release: () => release() }
}

/**
 * Builds the body of a refused answer.
 * @param code - The extensions code.
 * @param reason - The stable reason.
 * @param meta - The named values the reason's message fills in.
 * @returns The body carrying the refusal.
 */
export function refusal(code: string, reason: string, meta?: Record<string, unknown>) {
	return { errors: [{ message: 'refused', extensions: { code, reason, meta } }] }
}

/** Speaks every refused answer through the plugin's templates and core's contact_not_found. */
export function speakTemplates() {
	configureErrorText({
		templates: () => ({ ...errorTemplates(), contact_not_found: 'That contact no longer exists.' }),
		fallback: () => '',
	})
}

/**
 * Returns the first button one list item holds.
 * @param item - The list item.
 * @returns The button.
 */
export function firstButton(item: HTMLElement) {
	return within(item).getAllByRole('button')[0]
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
 * Answers the values query with whatever the returned setter last stored, so a test can change it mid flight.
 * @param first - The values answered until the setter is called.
 * @returns The setter.
 */
export function serveChangingValues(first: Record<string, unknown>) {
	let held = first
	server.use(
		graphql.query('ContactFieldValues', ({ variables }) =>
			HttpResponse.json({
				data: { contact: { __typename: 'Contact', id: variables.id, ...held } },
			}),
		),
	)
	return (next: Record<string, unknown>) => {
		held = next
	}
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
