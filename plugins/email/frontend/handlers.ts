// SPDX-License-Identifier: AGPL-3.0-or-later

import { graphql } from '@alphone/frontend-sdk/testing'
import { HttpResponse } from 'msw'

/** defaultThreadsPage is the threads a fixture page answers when the request names no limit. */
const defaultThreadsPage = 50

/** PageVariables are the limit and the offset a request for a page of threads names. */
export interface PageVariables {
	/** limit is how many threads the page holds, none for the size the server serves by default. */
	limit?: number | null
	/** offset is how many threads come before the page. */
	offset: number
}

/**
 * Builds the newest mail of a thread in graph shape.
 * @param id - The mail id.
 * @param fields - The fields overriding a whole inbound mail from Maria Perez.
 * @returns The mail the inbox document returns as a thread's latest.
 */
export function latestMail(id: string, fields: Record<string, unknown> = {}) {
	return {
		__typename: 'EmailMessage',
		id,
		from: { __typename: 'EmailAddress', name: 'Maria Perez', email: 'maria.perez@example.com' },
		snippet: '',
		direction: 'inbound',
		encrypted: false,
		signed: false,
		bodyState: 'whole',
		...fields,
	}
}

/**
 * Builds one thread in graph shape.
 * @param id - The thread id.
 * @param subject - The thread's subject.
 * @param fields - The fields overriding a thread of one mail with no file.
 * @returns The thread the inbox document returns.
 */
export function inboxThread(id: string, subject: string, fields: Record<string, unknown> = {}) {
	return {
		__typename: 'EmailThread',
		id,
		subject,
		lastMessageAt: '2026-11-20T10:00:00Z',
		messageCount: 1,
		hasAttachments: false,
		latest: latestMail(id),
		...fields,
	}
}

/**
 * Returns one page of the threads the way the server pages them, its own page for a request naming no limit.
 * @param held - Every thread the inbox holds, the newest first.
 * @param variables - The limit and the offset the request asks for.
 * @returns The inbox document body for that page.
 */
export function inboxPage(held: readonly Record<string, unknown>[], variables: PageVariables) {
	const limit = variables.limit ?? defaultThreadsPage
	return {
		data: {
			emailThreads: {
				__typename: 'EmailThreadPage',
				items: held.slice(variables.offset, variables.offset + limit),
				total: held.length,
				limit,
			},
		},
	}
}

/** mailbox is the mailbox the workspace holds. */
export const mailbox = {
	__typename: 'EmailMailbox',
	id: '019f6a00-0000-7000-8000-000000000001',
	address: 'info@example.com',
	aliases: [],
}

/** threads are the threads the inbox holds, the newest first. */
export const threads = [
	inboxThread('019f6a00-0000-7000-8000-000000000101', 'Delivery date', {
		lastMessageAt: '2026-11-20T10:00:00Z',
		latest: latestMail('019f6a00-0000-7000-8000-000000000201', {
			from: { __typename: 'EmailAddress', name: 'Ada Lovelace', email: 'ada@example.com' },
			snippet: 'The lamps leave the workshop on Monday.',
		}),
	}),
	inboxThread('019f6a00-0000-7000-8000-000000000102', 'Order confirmed', {
		lastMessageAt: '2026-11-20T00:00:00Z',
		hasAttachments: true,
		latest: latestMail('019f6a00-0000-7000-8000-000000000202', {
			snippet: 'I confirm the order of two wicker lamps.',
			signed: true,
		}),
	}),
	inboxThread('019f6a00-0000-7000-8000-000000000103', 'Site visit on Friday', {
		lastMessageAt: '2026-11-19T12:00:00Z',
		latest: latestMail('019f6a00-0000-7000-8000-000000000203', {
			snippet: 'Could you come by on Friday morning?',
		}),
	}),
	inboxThread('019f6a00-0000-7000-8000-000000000104', 'Payment details', {
		lastMessageAt: '2026-11-19T00:00:00Z',
		latest: latestMail('019f6a00-0000-7000-8000-000000000204', { encrypted: true }),
	}),
	inboxThread('019f6a00-0000-7000-8000-000000000105', 'Quote for the new lamp', {
		lastMessageAt: '2026-11-18T12:00:00Z',
		messageCount: 3,
		hasAttachments: true,
		latest: latestMail('019f6a00-0000-7000-8000-000000000205', {
			snippet: 'Here is a photo of the corner where the lamp goes.',
		}),
	}),
]

export const handlers = [
	graphql.query('EmailMailbox', () => HttpResponse.json({ data: { emailMailbox: mailbox } })),
	graphql.query('EmailInbox', ({ variables }) =>
		HttpResponse.json(inboxPage(threads, variables as PageVariables)),
	),
]
