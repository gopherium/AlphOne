// SPDX-License-Identifier: AGPL-3.0-or-later

import { seedSession } from '@gopherium/react-auth/testing'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { render, screen } from '@testing-library/react'
import { expect, test } from 'vitest'

import { MANAGE_USERS, can, useSession } from '../index'
import { adminSession, memberSession } from '../testing'

/** Reports what a plugin screen may do with the session it reads. */
function Probe() {
	const session = useSession()
	return (
		<ul>
			<li>{`manages:${String(can(session, MANAGE_USERS))}`}</li>
			<li>{`grantable:${(session?.grantable ?? []).join('|')}`}</li>
		</ul>
	)
}

/** Renders the probe with the given account seeded as the session. */
function renderWithSession(user: typeof adminSession | null) {
	const client = new QueryClient({
		defaultOptions: { queries: { retry: false, staleTime: Infinity } },
	})
	seedSession(client, user)
	render(
		<QueryClientProvider client={client}>
			<Probe />
		</QueryClientProvider>,
	)
}

test('an admin session manages users', () => {
	renderWithSession(adminSession)

	expect(screen.getByText('manages:true')).toBeInTheDocument()
})

test('a member session manages nothing', () => {
	renderWithSession(memberSession)

	expect(screen.getByText('manages:false')).toBeInTheDocument()
})

test('no session manages nothing', () => {
	renderWithSession(null)

	expect(screen.getByText('manages:false')).toBeInTheDocument()
})

test('a session carries the roles it may grant', () => {
	renderWithSession(adminSession)

	expect(screen.getByText('grantable:admin|member')).toBeInTheDocument()
})

test('a session from a server that sends none of it holds nothing', () => {
	const { role: _role, capabilities: _capabilities, grantable: _grantable, ...bare } = adminSession
	renderWithSession(bare as typeof adminSession)

	expect(screen.getByText('manages:false')).toBeInTheDocument()
	expect(screen.getByText('grantable:')).toBeInTheDocument()
})

test('a capability the session was never sent is refused', () => {
	renderWithSession(adminSession)

	expect(can(adminSession, 'manage_tenants')).toBe(false)
	expect(can(memberSession, MANAGE_USERS)).toBe(false)
	expect(can(null, MANAGE_USERS)).toBe(false)
	expect(can(undefined, MANAGE_USERS)).toBe(false)
})
