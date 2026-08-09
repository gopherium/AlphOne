// SPDX-License-Identifier: Elastic-2.0

/** Item is one record a node operation answered with. */
export type Item = Record<string, unknown>

/** Context carries the ids one case leaves for the cases after it. */
export interface Context {
	today: string
	contactID?: string
	taskID?: string
	importID?: string
	replayedTaskID?: string
}

/** Case is one operation the runbook drives and what it must answer with. */
export interface Case {
	/** name labels the case in the report. */
	name: string
	/** params builds the node parameters the case runs with. */
	params: (ctx: Context) => Record<string, unknown>
	/** check returns the complaint the answer earns, or null when it passed. */
	check?: (items: Item[], ctx: Context) => string | null
	/** expectFailure marks a case that passes only when the node fails. */
	expectFailure?: boolean
	/** repeat runs the case more than once, for the cases about replaying. */
	repeat?: number
}

/** taskKeys is the shape every task operation answered with under REST. */
const taskKeys = [
	'id',
	'assignee_id',
	'contact_id',
	'title',
	'status',
	'priority',
	'due_on',
	'origin_source',
	'origin_event_id',
	'created_at',
]

/** importKeys is the shape every import summary answered with under REST. */
const importKeys = [
	'id',
	'user_id',
	'filename',
	'state',
	'row_count',
	'imported_count',
	'skipped_count',
	'failed_count',
	'created_at',
]

/**
 * Returns one answered field as a string.
 * @param value - The field as it arrived.
 * @returns The field rendered for a complaint.
 */
function text(value: unknown): string {
	return typeof value === 'string' ? value : String(value)
}

/**
 * Returns the complaint when an item carries other than the wanted keys.
 * @param item - The answered record.
 * @param wanted - The keys the REST answer carried.
 * @returns The complaint, or null when the keys match.
 */
function hasExactly(item: Item, wanted: string[]): string | null {
	const got = Object.keys(item).sort().join(',')
	const want = [...wanted].sort().join(',')
	return got === want ? null : `keys ${got}, want ${want}`
}

/**
 * Returns the complaint when a case answered with other than one item.
 * @param items - The answered records.
 * @returns The complaint, or null when exactly one came back.
 */
function one(items: Item[]): string | null {
	return items.length === 1 ? null : `${items.length} items, want 1`
}

/** cases lists every operation the runbook drives, in the order it drives them. */
export const cases: Case[] = [
	{
		name: 'contact create',
		params: () => ({ resource: 'contact', operation: 'create', name: 'Maria Perez' }),
		check: (items, ctx) => {
			ctx.contactID = text(items[0]?.id)
			return (
				one(items) ??
				hasExactly(items[0], ['id', 'name', 'created_at']) ??
				(items[0].name === 'Maria Perez' ? null : `name ${text(items[0].name)}`)
			)
		},
	},
	{
		name: 'contact get many',
		params: () => ({ resource: 'contact', operation: 'getAll', limit: 200 }),
		check: (items) =>
			items.length === 0
				? 'no contacts, want the created one'
				: hasExactly(items[0], ['id', 'name', 'created_at']),
	},
	{
		name: 'contact get many, searched',
		params: () => ({ resource: 'contact', operation: 'getAll', search: 'Ada', limit: 200 }),
		check: (items) => {
			if (items.length === 0) {
				return 'no contacts matched, want the seeded Ada row'
			}
			return items.every((item) => text(item.name).includes('Ada'))
				? null
				: `matched ${items.map((item) => text(item.name)).join(', ')}, want only the Ada rows`
		},
	},
	{
		name: 'contact get',
		params: (ctx) => ({ resource: 'contact', operation: 'get', contactId: ctx.contactID }),
		check: (items) =>
			one(items) ?? hasExactly(items[0], ['id', 'name', 'created_at', 'identities']),
	},
	{
		name: 'contact get, unknown id',
		params: () => ({
			resource: 'contact',
			operation: 'get',
			contactId: '019f5a00-0000-7000-8000-000000000001',
		}),
		expectFailure: true,
	},
	{
		name: 'contact rename',
		params: (ctx) => ({
			resource: 'contact',
			operation: 'rename',
			contactId: ctx.contactID,
			name: 'Maria Lopez',
		}),
		check: (items) =>
			items[0]?.name === 'Maria Lopez' ? null : `name ${text(items[0]?.name)}`,
	},
	{
		name: 'task create',
		params: (ctx) => ({
			resource: 'task',
			operation: 'create',
			title: 'Call back',
			dueOn: ctx.today,
		}),
		check: (items, ctx) => {
			ctx.taskID = text(items[0]?.id)
			return (
				one(items) ??
				hasExactly(items[0], taskKeys) ??
				(items[0].origin_source === 'token:runbook'
					? null
					: `origin_source ${text(items[0].origin_source)}, want token:runbook`)
			)
		},
	},
	{
		name: 'task create, blank optional field',
		params: (ctx) => ({
			resource: 'task',
			operation: 'create',
			title: 'Call back again',
			dueOn: ctx.today,
			additionalFields: { contact_id: ctx.contactID, priority: 3, origin_event_id: '' },
		}),
		check: (items) => {
			const task = items[0] ?? {}
			if (task.contact_id && task.priority === 3 && task.origin_event_id === null) {
				return null
			}
			return (
				`contact_id ${text(task.contact_id)}, priority ${text(task.priority)}, ` +
				`origin_event_id ${text(task.origin_event_id)}`
			)
		},
	},
	{
		name: 'task create, replayed origin',
		params: (ctx) => ({
			resource: 'task',
			operation: 'create',
			title: 'Called once',
			dueOn: ctx.today,
			additionalFields: { origin_event_id: '019f5a00-0000-7000-8000-0000000000aa' },
		}),
		check: (items, ctx) => {
			const first = ctx.replayedTaskID
			ctx.replayedTaskID = text(items[0]?.id)
			return first === undefined || first === ctx.replayedTaskID
				? null
				: `the second create answered ${ctx.replayedTaskID}, want the stored ${first}`
		},
		repeat: 2,
	},
	{
		name: 'task get many, by day',
		params: (ctx) => ({
			resource: 'task',
			operation: 'getAll',
			filterBy: 'date',
			date: ctx.today,
			status: 'open',
			limit: 200,
		}),
		check: (items) =>
			items.length === 0 ? 'no tasks, want the created ones' : hasExactly(items[0], taskKeys),
	},
	{
		name: 'task get many, by contact',
		params: (ctx) => ({
			resource: 'task',
			operation: 'getAll',
			filterBy: 'contact_id',
			contactId: ctx.contactID,
			status: 'all',
			limit: 200,
		}),
		check: (items, ctx) =>
			items.every((item) => item.contact_id === ctx.contactID)
				? null
				: 'a task belonging to another contact came back',
	},
	{
		name: 'task get many, overdue',
		params: (ctx) => ({
			resource: 'task',
			operation: 'getAll',
			filterBy: 'due_before',
			dueBefore: ctx.today,
			status: 'open',
			limit: 200,
		}),
		check: (items, ctx) =>
			items.every((item) => item.status === 'open' && text(item.due_on) < ctx.today)
				? null
				: 'a task that is neither open nor overdue came back',
	},
	{
		name: 'task get',
		params: (ctx) => ({ resource: 'task', operation: 'get', taskId: ctx.taskID }),
		check: (items) => one(items) ?? hasExactly(items[0], taskKeys),
	},
	{
		name: 'task update, one field',
		params: (ctx) => ({
			resource: 'task',
			operation: 'update',
			taskId: ctx.taskID,
			updateFields: { title: 'Call back about the renewal' },
		}),
		check: (items) => {
			const task = items[0] ?? {}
			return task.title === 'Call back about the renewal' && task.status === 'open'
				? null
				: `title ${text(task.title)}, status ${text(task.status)}`
		},
	},
	{
		name: 'task update, no fields',
		params: (ctx) => ({ resource: 'task', operation: 'update', taskId: ctx.taskID }),
		check: (items) =>
			items[0]?.title === 'Call back about the renewal'
				? null
				: `title ${text(items[0]?.title)}, want it unchanged`,
	},
	{
		name: 'task complete',
		params: (ctx) => ({ resource: 'task', operation: 'complete', taskId: ctx.taskID }),
		check: (items) => (items[0]?.status === 'done' ? null : `status ${text(items[0]?.status)}`),
	},
	{
		name: 'import get many',
		params: () => ({ resource: 'import', operation: 'getAll' }),
		check: (items, ctx) => {
			ctx.importID = text(items[0]?.id)
			return items.length === 0
				? 'no imports, want the seeded one'
				: hasExactly(items[0], importKeys)
		},
	},
	{
		name: 'import get',
		params: (ctx) => ({ resource: 'import', operation: 'get', importId: ctx.importID }),
		check: (items) => one(items) ?? hasExactly(items[0], [...importKeys, 'columns', 'mapping']),
	},
	{
		name: 'import get contacts',
		params: (ctx) => ({ resource: 'import', operation: 'getContacts', importId: ctx.importID }),
		check: (items) =>
			items.length === 0
				? 'no contacts, want the imported ones'
				: hasExactly(items[0], ['contact_id', 'name', 'row_id']),
	},
]
