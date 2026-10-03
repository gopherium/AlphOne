// SPDX-License-Identifier: AGPL-3.0-or-later

import { GraphProvider, Toaster, createGraphClient } from '@alphone/frontend-sdk'
import { HttpResponse, graphql, server } from '@alphone/frontend-sdk/testing'
import { createAuthQueryClient } from '@gopherium/react-auth'
import { QueryClientProvider } from '@tanstack/react-query'
import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { expect, test, vi } from 'vitest'

import type { Account } from '../auth/graphTransport'
import { RoleModal } from '../users/UserModals'

vi.mock('@alphone/frontend-sdk', async (importOriginal) => ({
	...(await importOriginal<typeof import('@alphone/frontend-sdk')>()),
	SelectControl: ({ label, onValueChange }: { label: string, onValueChange: (item: null) => void }) => (
		<button type="button" onClick={() => onValueChange(null)}>{`Clear ${label}`}</button>
	),
}))

/** member is the account whose role the modal writes. */
const member: Account = {
	id: '0198b2f0-0000-7000-8000-0000000000ff',
	email: 'ada@example.com',
	name: 'Ada Lovelace',
	disabled: false,
	confirmed: true,
	created_at: new Date('2026-07-07T10:00:00Z'),
	role: 'member',
}

test('keeps the role the account holds when the select hands back no choice', async () => {
	let writes = 0
	server.use(
		graphql.mutation('SetUserRole', () => {
			writes += 1
			return HttpResponse.json({ data: { setUserRole: true } })
		}),
	)
	render(
		<QueryClientProvider client={createAuthQueryClient({ queries: { retry: false } })}>
			<GraphProvider graph={createGraphClient({ onSessionExpired: () => {} })}>
				<Toaster>
					<RoleModal items={[member]} closeModal={vi.fn()} grantable={['admin', 'member']} />
				</Toaster>
			</GraphProvider>
		</QueryClientProvider>,
	)

	await userEvent.click(screen.getByRole('button', { name: 'Clear Role' }))
	const change = screen.getByRole('button', { name: 'Change role' })
	expect(change).toHaveAttribute('aria-disabled', 'true')
	await userEvent.click(change)

	expect(writes).toBe(0)
})
