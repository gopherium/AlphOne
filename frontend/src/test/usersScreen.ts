// SPDX-License-Identifier: AGPL-3.0-or-later

import { HttpResponse, adminSession, graphql, server } from '@alphone/frontend-sdk/testing'
import { screen, waitFor, within } from '@testing-library/react'
import { expect } from 'vitest'

/** account is one user as the users query answers it. */
export interface account {
	__typename: 'User'
	id: string
	email: string
	name: string
	disabled: boolean
	confirmed: boolean
	createdAt: string
	role: string
	withinReach: boolean
}

/**
 * Builds one account the users query answers.
 * @param id - The last hex digits of its identifier.
 * @param name - Its display name.
 * @param changes - The fields that differ from an active member.
 * @returns The account.
 */
export function accountOf(id: string, name: string, changes: Partial<account> = {}): account {
	return {
		__typename: 'User',
		id: `0198b2f0-0000-7000-8000-0000000000${id}`,
		email: `${name.split(' ')[0].toLowerCase()}@example.com`,
		name,
		disabled: false,
		confirmed: true,
		createdAt: '2026-07-07T10:00:00Z',
		role: 'member',
		withinReach: true,
		...changes,
	}
}

/** signedIn is the administrator account the tests sign in as. */
export const signedIn = accountOf('01', adminSession.name, {
	id: adminSession.id,
	email: adminSession.email,
	role: 'admin',
	createdAt: '2026-07-06T10:00:00Z',
})
/** colleague is an active member account the signed-in administrator may change. */
export const colleague = accountOf('ff', 'Ada Lovelace')
/** barred is a disabled member account, the newest of the three. */
export const barred = accountOf('fe', 'Ana Lopez', { disabled: true, createdAt: '2026-07-08T10:00:00Z' })

/**
 * Serves from the users query the accounts the given read returns at each request.
 * @param read - Returns the accounts the list holds.
 */
export function serveUsers(read: () => account[]) {
	server.use(graphql.query('Users', () => HttpResponse.json({ data: { users: read() } })))
}

/**
 * Returns the table row holding an element whose whole text is the given text.
 * @param text - A name or address the row shows.
 * @returns The row queries.
 */
export async function rowOf(text: string) {
	return within(
		await waitFor(() => {
			const row = screen.getAllByRole('row').find((candidate) => within(candidate).queryByText(text) !== null)
			expect(row).toBeDefined()
			return row as HTMLElement
		}),
	)
}
