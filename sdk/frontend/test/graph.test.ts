// SPDX-License-Identifier: AGPL-3.0-or-later

import { UnauthorizedError } from '@gopherium/react-auth'
import { gql } from 'urql'
import { afterEach, expect, onTestFinished, test, vi } from 'vitest'
import { pipe, subscribe } from 'wonka'

import { ValidationError, validationMessage } from '../errors'
import { rememberFormatLocale } from '../format'
import { configureErrorText, createGraphClient, graphError, graphExtensions, reasonText } from '../graph'
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

afterEach(() => {
	configureErrorText({ templates: () => ({}), fallback: () => '' })
})

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

test('keeps an ordinary error a call threw as it is', () => {
	const thrown = new Error('stream ended')

	expect(graphError(thrown)).toBe(thrown)
})

test('turns a thrown value that is no error into an error carrying its text', () => {
	expect(graphError('stream ended')).toEqual(new Error('stream ended'))
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
			rows {
				id
				reason {
					code
					meta
				}
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

const fieldsQuery = gql`
	query Fields {
		fields {
			id
			subFields {
				name
				label
				kind
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

const adminSettingsQuery = gql`
	query AdminSettings {
		adminSettings {
			toastMilliseconds
			listPageSizes
		}
	}
`

test('keys every embedded type the graph returns without warning', async () => {
	const warn = vi.spyOn(console, 'warn').mockImplementation(() => {})
	server.use(
		graphql.query('AdminSettings', () =>
			HttpResponse.json({
				data: {
					adminSettings: { __typename: 'AdminSettings', toastMilliseconds: 6000, listPageSizes: [10, 20] },
				},
			}),
		),
		graphql.query('ImportJob', () =>
			HttpResponse.json({
				data: {
					importJob: {
						__typename: 'ImportJob',
						id: 'id-import',
						mapping: [{ __typename: 'ImportAssignment', column: 0, field: 'name' }],
						rows: [
							{
								__typename: 'ImportRow',
								id: 'id-row',
								reason: {
									__typename: 'ImportRowReason',
									code: 'identity_taken_by',
									meta: { ownerName: 'Maria Perez' },
								},
							},
						],
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
		graphql.query('Fields', () =>
			HttpResponse.json({
				data: {
					fields: [
						{
							__typename: 'FieldDefinition',
							id: 'id-history',
							subFields: [
								{ __typename: 'FieldSubField', name: 'date', label: 'Date', kind: 'DATE' },
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
		await graph.client.query(fieldsQuery, {}).toPromise(),
		await graph.client.mutation(createTaskMutation, { input: { title: 'x', dueOn: '2026-08-07' } }).toPromise(),
		await graph.client.mutation(importCommitMutation, { id: 'id-import' }).toPromise(),
		await graph.client
			.mutation(loginMutation, { email: 'maria@example.com', password: 'password1234' })
			.toPromise(),
		await graph.client
			.mutation(createWebhookMutation, { url: 'https://example.com/hook', events: ['task.created'] })
			.toPromise(),
		await graph.client.query(adminSettingsQuery, {}).toPromise(),
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

test('renders a refused answer from the template its reason names', async () => {
	configureErrorText({
		templates: () => ({ contact_name_required: 'Escribe un nombre.' }),
		fallback: () => 'Algo salio mal.',
	})
	server.use(
		graphql.query('Version', () =>
			HttpResponse.json({
				data: null,
				errors: [{
					message: 'contact: empty name',
					extensions: { code: 'VALIDATION', reason: 'contact_name_required' },
				}],
			})),
	)
	const { graph } = newClient()

	const result = await graph.client.query(versionQuery, {}).toPromise()

	expect(graphError(result.error)?.message).toBe('Escribe un nombre.')
})

test('fills a template from the numbers the answer carries, written in the format locale', async () => {
	configureErrorText({
		templates: () => ({ first_out_of_range: 'Pide entre %(min)s y %(max)s cada vez.' }),
		fallback: () => 'Algo salio mal.',
	})
	server.use(
		graphql.query('Version', () =>
			HttpResponse.json({
				data: null,
				errors: [{
					message: 'graph: first must be between 1 and 1250',
					extensions: {
						code: 'VALIDATION',
						reason: 'first_out_of_range',
						meta: { min: 1, max: 1250 },
					},
				}],
			})),
	)
	const { graph } = newClient()

	const result = await graph.client.query(versionQuery, {}).toPromise()

	expect(graphError(result.error)?.message).toBe('Pide entre 1 y 1.250 cada vez.')
})

test('fills a template from the text the answer carries as it came', async () => {
	configureErrorText({
		templates: () => ({ scope_missing: 'Este token no alcanza %(scope)s.' }),
		fallback: () => 'Algo salio mal.',
	})
	server.use(
		graphql.query('Version', () =>
			HttpResponse.json({
				data: null,
				errors: [{
					message: 'graph: the token does not reach contacts:write',
					extensions: { code: 'FORBIDDEN', reason: 'scope_missing', meta: { scope: 'contacts:write' } },
				}],
			})),
	)
	const { graph } = newClient()

	const result = await graph.client.query(versionQuery, {}).toPromise()

	expect(graphError(result.error)?.message).toBe('Este token no alcanza contacts:write.')
})

/**
 * Answers every graph post with a refusal the router sent before the graph read it.
 * @param body - The answer body.
 * @param contentType - The media type the answer names.
 * @param status - The HTTP status the answer carries.
 */
function refuseAtTheRouter(body: string, contentType: string, status = 403) {
	server.use(
		http.post('/api/graphql', () =>
			new HttpResponse(body, { status, headers: { 'Content-Type': contentType } })),
	)
}

/** The answer the router sends a browser write it judged cross-origin. */
const crossOriginRefusal = '{"error":"cross-origin request refused","code":"request_cross_origin"}'

test('speaks the template a router refusal names by its code', async () => {
	configureErrorText({
		templates: () => ({ request_cross_origin: 'Cambio rechazado desde otro sitio.' }),
		fallback: () => 'Algo salio mal.',
	})
	refuseAtTheRouter(crossOriginRefusal, 'application/json')
	const { graph } = newClient()

	const result = await graph.client.mutation(createTaskMutation, { input: { title: 'x', dueOn: '2026-08-07' } })
		.toPromise()

	expect(graphError(result.error)).toBeInstanceOf(ValidationError)
	expect(validationMessage(graphError(result.error), 'No se ha podido añadir la tarea.'))
		.toBe('Cambio rechazado desde otro sitio.')
	expect(graphExtensions(result.error)).toEqual({ code: 'VALIDATION', reason: 'request_cross_origin' })
	expect((result.error?.response as Response | undefined)?.status).toBe(403)
})

test('leaves a coded answer on another status as the failure it is', async () => {
	for (const [status, body] of [
		[401, '{"error":"no session","code":"session_absent"}'],
		[429, '{"error":"too many login attempts, try again later","code":"login_rate_limited"}'],
	] as const) {
		refuseAtTheRouter(body, 'application/json', status)
		const { graph } = newClient()

		const result = await graph.client.mutation(createTaskMutation, { input: { title: 'x', dueOn: '2026-08-07' } })
			.toPromise()

		expect(result.error?.graphQLErrors).toEqual([])
		expect((result.error?.response as Response | undefined)?.status).toBe(status)
	}
})

test('leaves a refusal naming a code beside no message as the failure it is', async () => {
	refuseAtTheRouter('{"code":"request_cross_origin"}', 'application/json')
	const { graph } = newClient()

	const result = await graph.client.query(versionQuery, {}).toPromise()

	expect(result.error?.graphQLErrors).toEqual([])
})

test('leaves a refusal sent as another media type as the failure it is', async () => {
	refuseAtTheRouter(crossOriginRefusal, 'text/plain')
	const { graph } = newClient()

	const result = await graph.client.query(versionQuery, {}).toPromise()

	expect(result.error?.graphQLErrors).toEqual([])
})

test('speaks the router message for a refusal code no template holds', async () => {
	configureErrorText({ templates: () => ({}), fallback: () => 'Algo salio mal.' })
	refuseAtTheRouter(crossOriginRefusal, 'application/json; charset=utf-8')
	const { graph } = newClient()

	const result = await graph.client.query(versionQuery, {}).toPromise()

	expect(graphError(result.error)?.message).toBe('cross-origin request refused')
})

test('leaves a router refusal naming no code as the failure it is', async () => {
	refuseAtTheRouter('{"error":"tenant deactivated"}', 'application/json')
	const { graph } = newClient()

	const result = await graph.client.query(versionQuery, {}).toPromise()

	expect(result.error?.graphQLErrors).toEqual([])
	expect((result.error?.response as Response | undefined)?.status).toBe(403)
})

test('leaves a refusal that is no JSON as the failure it is', async () => {
	refuseAtTheRouter('forbidden', 'text/plain')
	const { graph } = newClient()

	const result = await graph.client.query(versionQuery, {}).toPromise()

	expect(result.error?.graphQLErrors).toEqual([])
})

test('leaves a refusal whose JSON cannot be read as the failure it is', async () => {
	refuseAtTheRouter('{"error":', 'application/json')
	const { graph } = newClient()

	const result = await graph.client.query(versionQuery, {}).toPromise()

	expect(result.error?.graphQLErrors).toEqual([])
})

test('reads a refusal the graph itself answered as its own errors', async () => {
	refuseAtTheRouter('{"errors":[{"message":"graph: refused","extensions":{"code":"FORBIDDEN"}}]}', 'application/json')
	const { graph } = newClient()

	const result = await graph.client.query(versionQuery, {}).toPromise()

	expect(graphError(result.error)?.message).toBe('graph: refused')
	expect(graphExtensions(result.error)).toEqual({ code: 'FORBIDDEN' })
})

test('speaks the server message for a reason no template holds', async () => {
	configureErrorText({ templates: () => ({}), fallback: () => 'Algo salio mal.' })
	respondWithError('Version', 'VALIDATION', 'a message from the future')
	const { graph } = newClient()

	const result = await graph.client.query(versionQuery, {}).toPromise()

	expect(graphError(result.error)?.message).toBe('a message from the future')
})

test('renders a stored reason from the template its code names', () => {
	const templates = { identity_taken_by: '%(ownerName)s ya tiene una dirección de esta fila.' }

	expect(reasonText({ code: 'identity_taken_by', meta: { ownerName: 'Maria Perez' } }, templates, 'Sin motivo.'))
		.toBe('Maria Perez ya tiene una dirección de esta fila.')
})

test('writes the numbers a stored reason carries in the format locale', () => {
	rememberFormatLocale('es-ES')
	onTestFinished(() => rememberFormatLocale(undefined))
	const templates = { row_cell_count_mismatch: 'Celdas en la fila: %(cells)s. Columnas: %(columns)s.' }

	expect(reasonText({ code: 'row_cell_count_mismatch', meta: { cells: 1234, columns: 3 } }, templates, 'Sin motivo.'))
		.toBe('Celdas en la fila: 1.234. Columnas: 3.')
})

test('speaks the fallback for a code no template holds', () => {
	expect(reasonText({ code: 'row_from_the_future' }, {}, 'row_from_the_future')).toBe('row_from_the_future')
})

test('speaks the fallback when a stored reason lacks a value its template names', () => {
	const templates = { identity_taken_by: '%(ownerName)s ya tiene una dirección de esta fila.' }

	expect(reasonText({ code: 'identity_taken_by', meta: {} }, templates, 'identity_taken_by')).toBe('identity_taken_by')
	expect(reasonText({ code: 'identity_taken_by', meta: null }, templates, 'identity_taken_by')).toBe('identity_taken_by')
})

const contactPageQuery = gql`
	query ContactPage($limit: Int) {
		contactPage(limit: $limit) {
			items {
				id
				name
			}
			total
			limit
		}
	}
`

test('keeps a page of contacts embedded in its query without warning', async () => {
	const warn = vi.spyOn(console, 'warn').mockImplementation(() => {})
	server.use(
		graphql.query('ContactPage', () =>
			HttpResponse.json({
				data: {
					contactPage: {
						__typename: 'ContactPage',
						items: [{ __typename: 'Contact', id: 'id-maria', name: 'Maria Perez' }],
						total: 1,
						limit: 20,
					},
				},
			}),
		),
	)
	const { graph } = newClient()

	const result = await graph.client.query(contactPageQuery, { limit: 20 }).toPromise()

	expect(result.data?.contactPage.total).toBe(1)
	expect(warn.mock.calls.flat().join('\n')).not.toContain('ContactPage')
})
