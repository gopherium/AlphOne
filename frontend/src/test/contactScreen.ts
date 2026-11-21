// SPDX-License-Identifier: AGPL-3.0-or-later

import { HttpResponse, graphql, server } from '@alphone/frontend-sdk/testing'
import { screen } from '@testing-library/react'
import { expect } from 'vitest'

/** anaID is the id of the contact Ana García a test opens. */
export const anaID = '0198c000-0000-7000-8000-000000000001'
/** brunoID is the id of the contact Bruno, who holds a value or is opened after Ana García. */
export const brunoID = '0198c000-0000-7000-8000-000000000002'
/** identityID1 is the id of the first identity a contact holds. */
export const identityID1 = '0198c000-0000-7000-8000-000000000011'
/** identityID2 is the id of the second identity a contact holds. */
export const identityID2 = '0198c000-0000-7000-8000-000000000012'

/** IdentityRow is one identity as the mocked server stores it. */
export type IdentityRow = { id: string; channel: string; identifier: string; display_name: string }

/**
 * Returns the contact node the detail query answers, with no tasks.
 * @param id - The id of the contact.
 * @param name - The name of the contact.
 * @param identities - The identities the contact holds.
 * @returns The contact node.
 */
export function detailFor(id: string, name: string, identities: IdentityRow[]) {
	return {
		__typename: 'Contact',
		id,
		name,
		createdAt: '2026-07-06T10:00:00Z',
		identities: identities.map((identity) => ({
			__typename: 'ContactIdentity',
			id: identity.id,
			channel: identity.channel,
			identifier: identity.identifier,
			displayName: identity.display_name,
		})),
		tasks: {
			__typename: 'TaskConnection',
			edges: [],
			pageInfo: { __typename: 'PageInfo', hasNextPage: false, endCursor: null },
		},
	}
}

/**
 * Returns the direct children of the form row that holds the named button.
 * @param button - The name of the row's submit button.
 * @returns The row's children, in order.
 */
export function formRowChildren(button: string) {
	const row = screen.getByRole('button', { name: button }).parentElement
	expect(row).toHaveClass('godmin-form__row')
	expect(row?.parentElement).toHaveClass('godmin-form')
	return [...(row as HTMLElement).children]
}

/**
 * Expects one row child to hold both the labelled control and its label.
 * @param child - The direct child of the form row.
 * @param label - The text of the control's label.
 */
export function expectFieldIn(child: Element | undefined, label: string) {
	expect(child).toContainElement(screen.getByLabelText(label))
	expect(child).toContainElement(screen.getByText(label))
}

/**
 * Serves every contact as Ana García with two WhatsApp identities and loads the contact screens.
 */
export async function serveAnaGarcia() {
	server.use(
		graphql.query('ContactDetail', ({ variables }) =>
			HttpResponse.json({
				data: {
					contact: detailFor(String(variables.id), 'Ana García', [
						{ id: identityID1, channel: 'whatsapp', identifier: '184467235', display_name: 'Ana G' },
						{ id: identityID2, channel: 'whatsapp', identifier: '184467236', display_name: '' },
					]),
				},
			}),
		),
	)
	await import('../contacts/ContactScreen')
	await import('../contacts/ContactsScreen')
}
