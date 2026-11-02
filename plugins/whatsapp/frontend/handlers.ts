// SPDX-License-Identifier: AGPL-3.0-or-later

import { graphql } from '@alphone/frontend-sdk/testing'
import { HttpResponse } from 'msw'

/**
 * Builds one thread message in graph shape.
 * @param id - The message id.
 * @param externalId - The WhatsApp message id.
 * @param fields - The fields overriding the inbound text defaults.
 * @returns The message the thread document returns.
 */
function threadMessage(id: string, externalId: string, fields: Record<string, unknown>) {
	return {
		__typename: 'WhatsAppMessage',
		id,
		externalId,
		direction: 'inbound',
		contentType: 'text',
		status: null,
		statusDetail: null,
		media: null,
		...fields,
	}
}

/**
 * Builds one connected number in graph shape.
 * @param identifier - The number id.
 * @param label - The label the workspace gave it.
 * @returns The number the numbers document returns.
 */
function connectedNumber(identifier: string, label: string) {
	return { __typename: 'WhatsAppNumber', identifier, label }
}

export const conversationID = '019f4a00-0000-7000-8000-000000000001'

/** twoNumbers serves a workspace holding two numbers, the second one without a label. */
export const twoNumbers = graphql.query('WhatsAppNumbers', () =>
	HttpResponse.json({
		data: { whatsAppNumbers: [connectedNumber('5550001', 'Sales line'), connectedNumber('5550002', '')] },
	}),
)

const conversations = [
	{
		__typename: 'WhatsAppConversation',
		id: conversationID,
		status: 'open',
		lastActivityAt: '2026-07-06T10:05:00Z',
		lastMessagePreview: 'I can pick it up after 5pm.',
		number: 'Sales line',
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
		number: '5550002',
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
		number: null,
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
	graphql.query('WhatsAppNumbers', () =>
		HttpResponse.json({ data: { whatsAppNumbers: [connectedNumber('5550001', 'Sales line')] } }),
	),
	graphql.query('WhatsAppThread', () =>
		HttpResponse.json({
			data: {
				whatsAppConversation: {
					__typename: 'WhatsAppConversation',
					id: conversations[0].id,
					number: conversations[0].number,
					contact: conversations[0].contact,
					messages: [
						threadMessage('019f4a00-0000-7000-8000-0000000000b1', 'wamid.HBgLMTU1NTAwMDExMQ', {
							content: 'Hi, is the order ready?',
							sentAt: '2026-07-06T09:05:00Z',
						}),
						threadMessage('019f4a00-0000-7000-8000-0000000000b2', 'wamid.HBgLMTU1NTAwMDExMg', {
							content: 'I can pick it up after 5pm.',
							sentAt: '2026-07-06T10:05:00Z',
						}),
					],
				},
			},
		}),
	),
]
