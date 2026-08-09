// SPDX-License-Identifier: AGPL-3.0-or-later

import { execFile } from 'node:child_process'
import { createHmac } from 'node:crypto'
import { createServer } from 'node:http'
import type { AddressInfo } from 'node:net'
import { join } from 'node:path'
import { promisify } from 'node:util'

import { expect, test } from '@playwright/test'
import type { APIRequestContext } from '@playwright/test'

import { createTask, graph } from '../graph'
import { baseURL, credentials, databaseURL, repoRoot } from '../env'

const run = promisify(execFile)

/** One webhook delivery as the receiver saw it. */
type Delivery = { body: string; signature: string }

/** The receiver AlphOne delivers to, and the deliveries it collected. */
type Receiver = { url: string; deliveries: Delivery[]; close: () => Promise<void> }

/**
 * Mints an API token for the seeded user.
 * @param name - The token name, which becomes the origin source of its tasks.
 * @returns The token secret.
 */
async function mintToken(name: string): Promise<string> {
	const { stdout } = await run(
		join(repoRoot, 'alphone'),
		['token', 'create', '-email', credentials.email, '-name', name],
		{ env: { ...process.env, ALPHONE_DATABASE_URL: databaseURL } },
	)
	const secret = /^secret: (.+)$/m.exec(stdout)?.[1]
	if (!secret) {
		throw new Error(`token create printed no secret: ${stdout}`)
	}
	return secret
}

/**
 * Starts a local receiver collecting what AlphOne posts to it.
 * @returns The receiver, listening on a port of its own.
 */
async function startReceiver(): Promise<Receiver> {
	const deliveries: Delivery[] = []
	const server = createServer((request, response) => {
		let body = ''
		request.on('data', (chunk) => {
			body += String(chunk)
		})
		request.on('end', () => {
			const signature = request.headers['x-alphone-signature-256']
			deliveries.push({ body, signature: String(signature ?? '') })
			response.writeHead(204)
			response.end()
		})
	})
	await new Promise<void>((resolve) => server.listen(0, '127.0.0.1', resolve))
	const { port } = server.address() as AddressInfo
	return {
		url: `http://127.0.0.1:${port}/hook`,
		deliveries,
		close: () => new Promise<void>((resolve) => server.close(() => resolve())),
	}
}

/**
 * Uploads a file, maps its first two columns, and commits it.
 * @param api - The request context carrying the token.
 * @param csv - The file contents.
 * @returns The import id.
 */
async function commitImport(api: APIRequestContext, csv: string): Promise<string> {
	const upload = await api.post('/api/plugins/importer/imports', {
		multipart: {
			file: { name: 'leads.csv', mimeType: 'text/csv', buffer: Buffer.from(csv) },
		},
	})
	expect(upload.status(), await upload.text()).toBe(201)
	const { id } = (await upload.json()) as { id: string }
	const mapping = await api.put(`/api/plugins/importer/imports/${id}/mapping`, {
		data: {
			assignments: [
				{ column: 0, field: 'name' },
				{ column: 1, field: 'email' },
			],
		},
	})
	expect(mapping.status(), await mapping.text()).toBe(204)
	const commit = await api.post(`/api/plugins/importer/imports/${id}/commit`)
	expect(commit.status(), await commit.text()).toBe(200)
	return id
}

test('an import announces itself, and its rows create each task exactly once', async ({
	playwright,
}) => {
	const stamp = Date.now()
	const receiver = await startReceiver()
	const token = await mintToken(`e2e import ${stamp}`)
	const api = await playwright.request.newContext({
		baseURL,
		extraHTTPHeaders: { Authorization: `Bearer ${token}` },
	})

	const subscription = await graph<{
		createWebhook: { webhook: { id: string }; secret: string }
	}>(
		api,
		'mutation($url: String!, $events: [String!]!) {' +
			' createWebhook(url: $url, events: $events) { webhook { id } secret } }',
		{ url: receiver.url, events: ['import.completed'] },
	)
	const subscribed = { id: subscription.createWebhook.webhook.id, secret: subscription.createWebhook.secret }

	try {
		const csv = [
			'Name,Email',
			`Maria Perez ${stamp},maria.${stamp}@example.com`,
			`Ada Lovelace ${stamp},ada.${stamp}@example.com`,
			`Alan Turing ${stamp},alan.${stamp}@example.com`,
		].join('\n')
		const importID = await commitImport(api, csv)

		await expect
			.poll(() => receiver.deliveries.length, { timeout: 20_000 })
			.toBeGreaterThan(0)
		const delivered = receiver.deliveries[0]
		const expected = `sha256=${createHmac('sha256', subscribed.secret).update(delivered.body).digest('hex')}`
		expect(delivered.signature, 'the delivery is not signed with the subscription secret').toBe(
			expected,
		)
		const event = JSON.parse(delivered.body) as {
			event: string
			data: { id: string; imported: number; skipped: number }
		}
		expect(event.event).toBe('import.completed')
		expect(event.data).toMatchObject({ id: importID, imported: 3, skipped: 0 })

		const listed = await api.get(`/api/plugins/importer/imports/${importID}/contacts`)
		expect(listed.status(), await listed.text()).toBe(200)
		const { contacts } = (await listed.json()) as {
			contacts: { contact_id: string; name: string; row_id: string }[]
		}
		expect(contacts).toHaveLength(3)

		const dueOn = new Date(stamp + 30 * 24 * 60 * 60 * 1000).toISOString().slice(0, 10)
		for (const contact of contacts) {
			const created = await createTask(api, {
				title: `Call ${contact.name}`,
				dueOn,
				contactId: contact.contact_id,
				originEventId: contact.row_id,
			})
			expect(created.replay, `${contact.name} answered a replay on the first create`).toBe(false)
		}

		for (const contact of contacts) {
			const replayed = await createTask(api, {
				title: `Call ${contact.name}`,
				dueOn,
				contactId: contact.contact_id,
				originEventId: contact.row_id,
			})
			expect(replayed.replay, `${contact.name} created a second task for one row`).toBe(true)
		}

		for (const contact of contacts) {
			const listed = await graph<{ tasks: { edges: { node: { id: string } }[] } }>(
				api,
				'query($contactId: UUID!) { tasks(contactId: $contactId) { edges { node { id } } } }',
				{ contactId: contact.contact_id },
			)
			expect(
				listed.tasks.edges,
				`${contact.name} has more than the one task its row created`,
			).toHaveLength(1)
		}
	} finally {
		await graph(api, 'mutation($id: UUID!) { deleteWebhook(id: $id) }', { id: subscribed.id })
		await api.dispose()
		await receiver.close()
	}
})
