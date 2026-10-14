// SPDX-License-Identifier: AGPL-3.0-or-later

import { HttpResponse, fakeGraphClient, graphql, server } from '@alphone/frontend-sdk/testing'
import { expect, test, vi } from 'vitest'

import { plugin } from '../index'
import { threadQuery } from '../operations'

test('grants the area its routes are held to', () => {
	expect(plugin.tokenAreas).toEqual([{ value: 'whatsapp', label: 'WhatsApp' }])
})

test('keys the media of a message, which carries no id', () => {
	expect(Object.keys(plugin.graphKeys ?? {})).toEqual(['WhatsAppMedia'])
	expect(plugin.graphKeys?.WhatsAppMedia({})).toBeNull()
})

test('a client keying the plugin caches a thread with media without warning', async () => {
	const warn = vi.spyOn(console, 'warn').mockImplementation(() => {})
	server.use(
		graphql.query('WhatsAppThread', () =>
			HttpResponse.json({
				data: {
					whatsAppConversation: {
						__typename: 'WhatsAppConversation',
						id: 'id-conversation',
						contact: { __typename: 'Contact', id: 'id-contact', name: 'Maria Perez' },
						messages: [
							{
								__typename: 'WhatsAppMessage',
								id: 'id-message',
								externalId: 'wamid.declared',
								direction: 'in',
								content: '',
								contentType: 'image',
								sentAt: '2026-10-14T09:00:00Z',
								status: 'received',
								statusDetail: null,
								media: {
									__typename: 'WhatsAppMedia',
									status: 'stored',
									mimeType: 'image/jpeg',
									filename: null,
									fileSize: 2048,
									voice: false,
									animated: false,
									downloadPath: '/api/plugins/whatsapp/media',
								},
							},
						],
					},
				},
			}),
		),
	)
	const { graph } = fakeGraphClient([plugin])

	const result = await graph.client.query(threadQuery, { conversationId: 'id-conversation' }).toPromise()

	expect(result.error).toBeUndefined()
	expect(result.data?.whatsAppConversation?.messages[0]?.media?.status).toBe('stored')
	expect(warn).not.toHaveBeenCalled()
})
