// SPDX-License-Identifier: AGPL-3.0-or-later

import { HttpResponse, graphql, server } from '@alphone/frontend-sdk/testing'
import { screen, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { beforeEach, expect, test, vi } from 'vitest'

import { renderAt } from './render'

const sidebarChunk = vi.hoisted(() => {
	let markRequested = () => {}
	let land = () => {}
	const requested = new Promise<void>((resolve) => {
		markRequested = resolve
	})
	const landed = new Promise<void>((resolve) => {
		land = resolve
	})
	return { requested, landed, markRequested: () => markRequested(), land: () => land() }
})

vi.mock('../../../plugins/whatsapp/frontend/WhatsAppSidebar', async (importOriginal) => {
	sidebarChunk.markRequested()
	await sidebarChunk.landed
	return importOriginal()
})

beforeEach(() =>
	server.use(
		graphql.query('WhatsAppConversations', () =>
			HttpResponse.json({ data: { whatsAppConversations: [] } }),
		),
	),
)

test('keeps the main menu and the open screen until the WhatsApp sidebar has loaded', async () => {
	renderAt('/tasks')
	const nav = await screen.findByRole('navigation', { name: 'Navigation' })

	await userEvent.click(await within(nav).findByRole('link', { name: 'WhatsApp' }))
	await sidebarChunk.requested

	expect(within(nav).getByRole('link', { name: 'Tasks' })).toBeInTheDocument()
	expect(screen.getByRole('heading', { level: 1, name: 'Tasks' })).toBeInTheDocument()
	expect(screen.queryByText('No conversation selected.')).toBeNull()

	sidebarChunk.land()

	expect(await within(nav).findByRole('heading', { name: 'WhatsApp' })).toBeInTheDocument()
	expect(screen.getByText('No conversation selected.')).toBeInTheDocument()
	expect(within(nav).queryByRole('link', { name: 'Tasks' })).toBeNull()
})
