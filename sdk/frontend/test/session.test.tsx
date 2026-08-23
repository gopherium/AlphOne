// SPDX-License-Identifier: AGPL-3.0-or-later

import { seedSession } from '@gopherium/react-auth/testing'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { render, screen } from '@testing-library/react'
import { expect, test } from 'vitest'

import { useSession } from '../session'
import { adminSession, memberSession } from '../testing'

/** Prints the session a plugin screen would read, or that none is active. */
function Probe() {
	const session = useSession()
	if (!session) {
		return <p>signed out</p>
	}
	return (
		<p>
			{session.id} {session.email} {session.name} {session.role}
		</p>
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

test('answers the signed-in account with its tier', () => {
	renderWithSession(adminSession)

	const shown = screen.getByText(new RegExp(adminSession.email))
	expect(shown).toHaveTextContent(adminSession.id)
	expect(shown).toHaveTextContent(adminSession.name)
	expect(shown).toHaveTextContent('admin')
})

test('carries the canned member standing as a member', () => {
	renderWithSession(memberSession)

	expect(screen.getByText(new RegExp(memberSession.email))).toHaveTextContent('member')
})

test('answers null without a session', () => {
	renderWithSession(null)

	expect(screen.getByText('signed out')).toBeInTheDocument()
})

test('a session carries the role the server stored, whatever a plugin named it', () => {
	renderWithSession({ ...adminSession, role: 'steward' })

	expect(screen.getByText(/steward/)).toBeInTheDocument()
})
