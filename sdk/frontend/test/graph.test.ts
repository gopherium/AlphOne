// SPDX-License-Identifier: AGPL-3.0-or-later

import { UnauthorizedError } from '@gopherium/react-auth'
import { gql } from 'urql'
import { expect, test, vi } from 'vitest'
import { pipe, subscribe } from 'wonka'

import { ValidationError } from '../errors'
import { createGraphClient, graphError, graphExtensions } from '../graph'
import { HttpResponse, graphql, http, server } from '../testing'

const versionQuery = gql`
	query Version {
		version
	}
`

const contactsQuery = gql`
	query Contacts($first: Int, $after: String) {
		contacts(first: $first, after: $after) {
			edges {
				node {
					id
					name
				}
				cursor
			}
			pageInfo {
				hasNextPage
				endCursor
			}
		}
	}
`

const coreEventSubscription = gql`
	subscription CoreEvent {
		coreEvent
	}
`

const uploadMutation = gql`
	mutation ImportUpload($file: Upload!) {
		importUpload(file: $file) {
			id
			filename
		}
	}
`

/** Returns a client whose session expiry is observable. */
function newClient() {
	const onSessionExpired = vi.fn()
	return { graph: createGraphClient({ onSessionExpired }), onSessionExpired }
}

/** Answers one operation name with a graph errors envelope. */
function respondWithError(operation: string, code: string, message: string) {
	server.use(
		graphql.query(operation, () =>
			HttpResponse.json({ data: null, errors: [{ message, extensions: { code } }] }),
		),
	)
}

test('resolves a query against the graph endpoint with same origin credentials', async () => {
	const requests: Request[] = []
	server.use(
		graphql.query('Version', ({ request }) => {
			requests.push(request)
			return HttpResponse.json({ data: { version: '9.9.9' } })
		}),
	)
	const { graph } = newClient()

	const result = await graph.client.query(versionQuery, {}).toPromise()

	expect(result.error).toBeUndefined()
	expect(result.data).toEqual({ version: '9.9.9' })
	expect(requests).toHaveLength(1)
	expect(new URL(requests[0].url).pathname).toBe('/api/graphql')
	expect(requests[0].credentials).toBe('same-origin')
})

test('posts every operation rather than putting it in the url', async () => {
	const requests: Request[] = []
	server.use(
		graphql.query('Contacts', ({ request }) => {
			requests.push(request)
			return HttpResponse.json({
				data: {
					contacts: contactsPage(contactEdge('id-ada', 'Ada Lovelace', 'cursor-ada'), false, 'cursor-ada'),
				},
			})
		}),
	)
	const { graph } = newClient()

	await graph.client.query(contactsQuery, { first: 50 }).toPromise()

	expect(requests).toHaveLength(1)
	expect(requests[0].method).toBe('POST')
	expect(new URL(requests[0].url).search).toBe('')
})

test('clears the session when an operation answers UNAUTHENTICATED', async () => {
	respondWithError('Version', 'UNAUTHENTICATED', 'session expired')
	const { graph, onSessionExpired } = newClient()

	const result = await graph.client.query(versionQuery, {}).toPromise()

	expect(onSessionExpired).toHaveBeenCalledTimes(1)
	expect(graphError(result.error)).toBeInstanceOf(UnauthorizedError)
})

test('leaves the session alone when an operation succeeds', async () => {
	server.use(graphql.query('Version', () => HttpResponse.json({ data: { version: '9.9.9' } })))
	const { graph, onSessionExpired } = newClient()

	await graph.client.query(versionQuery, {}).toPromise()

	expect(onSessionExpired).not.toHaveBeenCalled()
})

test('maps a VALIDATION entry onto the shared validation error with its message', async () => {
	respondWithError('Version', 'VALIDATION', 'the title must not be empty')
	const { graph, onSessionExpired } = newClient()

	const result = await graph.client.query(versionQuery, {}).toPromise()

	const mapped = graphError(result.error)
	expect(mapped).toBeInstanceOf(ValidationError)
	expect(mapped?.message).toBe('the title must not be empty')
	expect(onSessionExpired).not.toHaveBeenCalled()
})

test('reports a network failure as an error rather than data', async () => {
	server.use(graphql.query('Version', () => HttpResponse.error()))
	const { graph } = newClient()

	const result = await graph.client.query(versionQuery, {}).toPromise()

	expect(result.data).toBeUndefined()
	expect(graphError(result.error)).toBeInstanceOf(Error)
	expect(graphError(result.error)).not.toBeInstanceOf(ValidationError)
})

test('maps an unclassified graph error onto a plain error carrying its message', async () => {
	respondWithError('Version', 'INTERNAL', 'internal error')
	const { graph } = newClient()

	const result = await graph.client.query(versionQuery, {}).toPromise()

	const mapped = graphError(result.error)
	expect(mapped).toBeInstanceOf(Error)
	expect(mapped).not.toBeInstanceOf(ValidationError)
	expect(mapped?.message).toBe('internal error')
})

test('reports no error for a result that carries none', () => {
	expect(graphError(undefined)).toBeUndefined()
})

test('maps a CONFLICT entry onto the shared validation error with its message', async () => {
	respondWithError('Version', 'CONFLICT', 'contact: identity already exists')
	const { graph } = newClient()

	const result = await graph.client.query(versionQuery, {}).toPromise()

	const mapped = graphError(result.error)
	expect(mapped).toBeInstanceOf(ValidationError)
	expect(mapped?.message).toBe('contact: identity already exists')
})

test('carries the extensions of a classified failure', async () => {
	server.use(
		graphql.query('Version', () =>
			HttpResponse.json({
				data: null,
				errors: [
					{
						message: 'contact: identity already exists',
						extensions: { code: 'CONFLICT', ownerName: 'Maria Perez' },
					},
				],
			}),
		),
	)
	const { graph } = newClient()

	const result = await graph.client.query(versionQuery, {}).toPromise()

	expect(graphExtensions(result.error)).toMatchObject({
		code: 'CONFLICT',
		ownerName: 'Maria Perez',
	})
})

test('carries no extensions when nothing failed', () => {
	expect(graphExtensions(undefined)).toEqual({})
})

/** Returns one contact edge as the graph serializes it. */
function contactEdge(id: string, name: string, cursor: string) {
	return { __typename: 'ContactEdge', node: { __typename: 'Contact', id, name }, cursor }
}

/** Returns one contacts page as the graph serializes it. */
function contactsPage(edge: object, hasNextPage: boolean, endCursor: string) {
	return {
		__typename: 'ContactConnection',
		edges: [edge],
		pageInfo: { __typename: 'PageInfo', hasNextPage, endCursor },
	}
}

test('pages a connection into one cached list rather than replacing it', async () => {
	server.use(
		graphql.query('Contacts', ({ variables }) =>
			HttpResponse.json({
				data: {
					contacts: variables.after === 'cursor-ada'
						? contactsPage(contactEdge('id-maria', 'Maria Perez', 'cursor-maria'), false, 'cursor-maria')
						: contactsPage(contactEdge('id-ada', 'Ada Lovelace', 'cursor-ada'), true, 'cursor-ada'),
				},
			}),
		),
	)
	const { graph } = newClient()

	await graph.client.query(contactsQuery, { first: 1 }).toPromise()
	const merged = await graph.client
		.query(contactsQuery, { first: 1, after: 'cursor-ada' })
		.toPromise()

	const page = merged.data?.contacts as {
		edges: { node: { name: string } }[]
		pageInfo: { hasNextPage: boolean }
	}
	expect(page.edges.map((edge) => edge.node.name)).toEqual(['Ada Lovelace', 'Maria Perez'])
	expect(page.pageInfo.hasNextPage).toBe(false)
})

const importJobQuery = gql`
	query ImportJob($id: UUID!) {
		importJob(id: $id) {
			id
			mapping {
				column
				field
			}
			contacts {
				contactId
				name
				rowId
			}
		}
		importFields {
			name
			label
			required
		}
	}
`

const conversationsQuery = gql`
	query WhatsAppConversations {
		whatsAppConversations {
			id
			messages {
				id
				media {
					status
					downloadPath
				}
			}
		}
	}
`

const createTaskMutation = gql`
	mutation CreateTask($input: CreateTaskInput!) {
		createTask(input: $input) {
			task {
				id
				title
			}
			replay
		}
	}
`

const importCommitMutation = gql`
	mutation ImportCommit($id: UUID!) {
		importCommit(id: $id) {
			id
			imported
		}
	}
`

const loginMutation = gql`
	mutation Login($email: String!, $password: String!) {
		login(email: $email, password: $password) {
			me {
				id
				email
			}
		}
	}
`

const createWebhookMutation = gql`
	mutation CreateWebhook($url: String!, $events: [String!]!) {
		createWebhook(url: $url, events: $events) {
			webhook {
				id
				url
			}
			secret
		}
	}
`

test('keys every embedded type the graph returns without warning', async () => {
	const warn = vi.spyOn(console, 'warn').mockImplementation(() => {})
	server.use(
		graphql.query('ImportJob', () =>
			HttpResponse.json({
				data: {
					importJob: {
						__typename: 'ImportJob',
						id: 'id-import',
						mapping: [{ __typename: 'ImportAssignment', column: 0, field: 'name' }],
						contacts: [
							{
								__typename: 'ImportContact',
								contactId: 'id-maria',
								name: 'Maria Perez',
								rowId: 'id-row',
							},
						],
					},
					importFields: [
						{ __typename: 'ImportField', name: 'name', label: 'Name', required: true },
					],
				},
			}),
		),
		graphql.query('WhatsAppConversations', () =>
			HttpResponse.json({
				data: {
					whatsAppConversations: [
						{
							__typename: 'WhatsAppConversation',
							id: 'id-conversation',
							messages: [
								{
									__typename: 'WhatsAppMessage',
									id: 'id-message',
									media: {
										__typename: 'WhatsAppMedia',
										status: 'stored',
										downloadPath: '/api/plugins/whatsapp/media',
									},
								},
							],
						},
					],
				},
			}),
		),
		graphql.mutation('CreateTask', () =>
			HttpResponse.json({
				data: {
					createTask: {
						__typename: 'CreateTaskPayload',
						task: { __typename: 'Task', id: 'id-task', title: 'Call Maria Perez' },
						replay: false,
					},
				},
			}),
		),
		graphql.mutation('ImportCommit', () =>
			HttpResponse.json({
				data: {
					importCommit: { __typename: 'ImportCommitPayload', id: 'id-import', imported: 2 },
				},
			}),
		),
		graphql.mutation('Login', () =>
			HttpResponse.json({
				data: {
					login: {
						__typename: 'LoginPayload',
						me: { __typename: 'Identity', id: 'id-maria', email: 'maria@example.com' },
					},
				},
			}),
		),
		graphql.mutation('CreateWebhook', () =>
			HttpResponse.json({
				data: {
					createWebhook: {
						__typename: 'CreateWebhookPayload',
						webhook: { __typename: 'Webhook', id: 'id-webhook', url: 'https://example.com/hook' },
						secret: 'synthetic-secret',
					},
				},
			}),
		),
	)
	const { graph } = newClient()

	const results = [
		await graph.client.query(importJobQuery, { id: 'id-import' }).toPromise(),
		await graph.client.query(conversationsQuery, {}).toPromise(),
		await graph.client.mutation(createTaskMutation, { input: { title: 'x', dueOn: '2026-08-07' } }).toPromise(),
		await graph.client.mutation(importCommitMutation, { id: 'id-import' }).toPromise(),
		await graph.client
			.mutation(loginMutation, { email: 'maria@example.com', password: 'password1234' })
			.toPromise(),
		await graph.client
			.mutation(createWebhookMutation, { url: 'https://example.com/hook', events: ['task.created'] })
			.toPromise(),
	]

	for (const result of results) {
		expect(result.error).toBeUndefined()
		expect(result.data).toBeTruthy()
	}
	expect(warn).not.toHaveBeenCalled()
})

/** Counts version requests and returns the counter beside the active subscription. */
function watchVersion() {
	let served = 0
	server.use(
		graphql.query('Version', () => {
			served += 1
			return HttpResponse.json({ data: { version: `9.9.${served}` } })
		}),
	)
	const { graph } = newClient()
	const versions: string[] = []
	const subscription = graph.client.query(versionQuery, {}).subscribe((result) => {
		if (result.data) {
			versions.push(result.data.version as string)
		}
	})
	return { graph, versions, subscription, served: () => served }
}

test('reruns an active query when the doorbell names its operation', async () => {
	const watch = watchVersion()
	await vi.waitFor(() => expect(watch.served()).toBe(1))

	watch.graph.refetch(['Version'])

	await vi.waitFor(() => expect(watch.served()).toBe(2))
	expect(watch.versions.at(0)).toBe('9.9.1')
	expect(watch.versions.at(-1)).toBe('9.9.2')
	watch.subscription.unsubscribe()
})

test('leaves an active query alone when the doorbell names another operation', async () => {
	const watch = watchVersion()
	await vi.waitFor(() => expect(watch.served()).toBe(1))

	watch.graph.refetch(['Contacts'])

	await new Promise((resolve) => setTimeout(resolve, 20))
	expect(watch.served()).toBe(1)
	watch.subscription.unsubscribe()
})

test('stops rerunning a query once nothing consumes it', async () => {
	const watch = watchVersion()
	await vi.waitFor(() => expect(watch.served()).toBe(1))
	watch.subscription.unsubscribe()

	watch.graph.refetch(['Version'])

	await new Promise((resolve) => setTimeout(resolve, 20))
	expect(watch.served()).toBe(1)
})

test('sends a file variable as a multipart request in the graph upload shape', async () => {
	let form: FormData | undefined
	const captureUpload: typeof fetch = (_input, init) => {
		form = init?.body as FormData
		return Promise.resolve(
			HttpResponse.json({
				data: { importUpload: { id: 'id-import', filename: 'contacts.csv' } },
			}),
		)
	}
	const { graph } = newClient()
	const file = new File(['Name,Email\nMaria Perez,maria@example.com\n'], 'contacts.csv', {
		type: 'text/csv',
	})

	const result = await graph.client
		.mutation(uploadMutation, { file }, { fetch: captureUpload })
		.toPromise()

	expect(result.error).toBeUndefined()
	expect(form).toBeInstanceOf(FormData)
	expect(form?.get('map')).toBe('{"0":["variables.file"]}')
	expect(typeof form?.get('0')).toBe('object')
	const operations = JSON.parse(String(form?.get('operations'))) as {
		query: string
		variables: Record<string, unknown>
	}
	expect(operations.query).toContain('importUpload')
	expect(operations.variables).toEqual({ file: null })
})

test('delivers subscription frames over the event stream transport', async () => {
	const requests: Request[] = []
	server.use(
		http.post('/api/graphql', ({ request }) => {
			requests.push(request)
			return new HttpResponse(
				'event: next\ndata: {"data":{"coreEvent":"task.created"}}\n\nevent: complete\n\n',
				{ headers: { 'content-type': 'text/event-stream' } },
			)
		}),
	)
	const { graph } = newClient()

	const frame = await new Promise<unknown>((resolve) => {
		pipe(
			graph.client.subscription(coreEventSubscription, {}),
			subscribe((result) => resolve(result.data)),
		)
	})

	expect(frame).toEqual({ coreEvent: 'task.created' })
	expect(requests).toHaveLength(1)
	expect(requests[0].headers.get('accept')).toContain('text/event-stream')
})

test('announces every event stream connection to its listeners', async () => {
	server.use(
		http.post('/api/graphql', () =>
			new HttpResponse('event: complete\n\n', {
				headers: { 'content-type': 'text/event-stream' },
			}),
		),
	)
	const { graph } = newClient()
	const opened = vi.fn()
	graph.onStreamOpen(opened)

	await new Promise<void>((resolve) => {
		pipe(
			graph.client.subscription(coreEventSubscription, {}),
			subscribe({ complete: () => resolve() } as never),
		)
		setTimeout(resolve, 500)
	})

	expect(opened).toHaveBeenCalled()
})

test('reconnects after the event stream drops', async () => {
	const requests: Request[] = []
	server.use(
		http.post('/api/graphql', ({ request }) => {
			requests.push(request)
			return new HttpResponse('', { headers: { 'content-type': 'text/event-stream' } })
		}),
	)
	const { graph } = newClient()
	const subscription = pipe(
		graph.client.subscription(coreEventSubscription, {}),
		subscribe(() => {}),
	)

	await vi.waitFor(() => expect(requests.length).toBeGreaterThan(1), { timeout: 4_000 })

	subscription.unsubscribe()
})
