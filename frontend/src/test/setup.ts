// SPDX-License-Identifier: AGPL-3.0-or-later

import '@testing-library/jest-dom/vitest'
import { cleanup } from '@testing-library/react'
import { HttpResponse, http } from 'msw'
import { setupServer } from 'msw/node'
import { afterAll, afterEach, beforeAll, vi } from 'vitest'

vi.stubGlobal('scrollTo', () => {})

afterEach(() => cleanup())

export const server = setupServer(
	http.get('/api/plugins/whatsapp/conversations', () =>
		HttpResponse.json([
			{
				id: '019f4a00-0000-7000-8000-000000000001',
				contact_id: '019f4a00-0000-7000-8000-0000000000a1',
				contact_name: 'John Doe',
				external_id: '555000111',
				status: 'open',
				last_activity_at: '2026-07-06T10:05:00Z',
			},
			{
				id: '019f4a00-0000-7000-8000-000000000002',
				contact_id: '019f4a00-0000-7000-8000-0000000000a2',
				contact_name: 'María Pérez',
				external_id: '184467235',
				status: 'open',
				last_activity_at: '2026-07-06T10:00:00Z',
			},
		]),
	),
	http.get('/api/plugins/whatsapp/conversations/:conversationId/messages', () =>
		HttpResponse.json([
			{
				id: '019f4a00-0000-7000-8000-0000000000b1',
				external_id: 'wamid.HBgLMTU1NTAwMDExMQ',
				direction: 'inbound',
				content: 'Hi, is the order ready?',
				content_type: 'text',
				sent_at: '2026-07-06T10:00:00Z',
			},
			{
				id: '019f4a00-0000-7000-8000-0000000000b2',
				external_id: 'wamid.HBgLMTU1NTAwMDExMg',
				direction: 'inbound',
				content: 'I can pick it up after 5pm.',
				content_type: 'text',
				sent_at: '2026-07-06T10:05:00Z',
			},
		]),
	),
)

beforeAll(() => server.listen({ onUnhandledRequest: 'error' }))
afterEach(() => server.resetHandlers())
afterAll(() => server.close())
