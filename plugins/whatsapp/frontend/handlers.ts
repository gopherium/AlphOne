// SPDX-License-Identifier: AGPL-3.0-or-later

import { HttpResponse, graphql, http } from 'msw'

const conversations = [
	{
		__typename: 'WhatsAppConversation',
		id: '019f4a00-0000-7000-8000-000000000001',
		status: 'open',
		lastActivityAt: '2026-07-06T10:05:00Z',
		lastMessagePreview: 'I can pick it up after 5pm.',
		contact: {
			__typename: 'Contact',
			id: '019f4a00-0000-7000-8000-0000000000a1',
			name: 'John Doe',
		},
	},
	{
		__typename: 'WhatsAppConversation',
		id: '019f4a00-0000-7000-8000-000000000002',
		status: 'open',
		lastActivityAt: '2026-07-06T10:00:00Z',
		lastMessagePreview: 'hello',
		contact: {
			__typename: 'Contact',
			id: '019f4a00-0000-7000-8000-0000000000a2',
			name: 'María Pérez',
		},
	},
	{
		__typename: 'WhatsAppConversation',
		id: '019f4a00-0000-7000-8000-000000000003',
		status: 'open',
		lastActivityAt: '2026-07-05T09:00:00Z',
		lastMessagePreview: null,
		contact: {
			__typename: 'Contact',
			id: '019f4a00-0000-7000-8000-0000000000a3',
			name: 'Quiet Contact',
		},
	},
]

export const handlers = [
	graphql.query('WhatsAppConversations', () =>
		HttpResponse.json({ data: { whatsAppConversations: conversations } }),
	),
	http.get('/api/plugins/whatsapp/conversations/:conversationId/messages', () =>
		HttpResponse.json([
			{
				id: '019f4a00-0000-7000-8000-0000000000b1',
				external_id: 'wamid.HBgLMTU1NTAwMDExMQ',
				direction: 'inbound',
				content: 'Hi, is the order ready?',
				content_type: 'text',
				sent_at: '2026-07-06T09:05:00Z',
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
]
