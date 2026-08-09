// SPDX-License-Identifier: AGPL-3.0-or-later

import { expect, test } from '@playwright/test'

import { subscribed } from '../subscription'

// answer is one graph response beside the operation that asked for it.
type answer = { operation: string; contentType: string }

/**
 * Returns whether a posted graph body asks for a subscription.
 * @param body - The request body, or null when Playwright withheld it.
 * @returns Whether the operation is a subscription.
 */
function isSubscription(body: string | null): boolean {
	return /"query"\s*:\s*"\s*subscription/.test(body ?? '')
}

/**
 * Returns the operation name a posted graph body carries.
 * @param body - The request body, or null when Playwright withheld it.
 * @returns The name, or the body itself when it carries none.
 */
function operationOf(body: string | null): string {
	return /(?:query|mutation)\s+(\w+)/.exec(body ?? '')?.[1] ?? String(body).slice(0, 40)
}

test('answers every read with JSON, leaving the stream budget to subscriptions', async ({
	page,
}) => {
	const answers: answer[] = []
	page.on('response', async (response) => {
		if (!response.url().includes('/api/graphql')) {
			return
		}
		const body = response.request().postData()
		if (isSubscription(body)) {
			return
		}
		answers.push({
			operation: operationOf(body),
			contentType: response.headers()['content-type'] ?? '',
		})
	})

	const stream = subscribed(page, 'coreEvent')
	await page.goto('/')
	await stream
	await expect(page.getByRole('heading', { name: 'Tasks' })).toBeVisible()

	expect(answers.length, 'the page read something over the graph').toBeGreaterThan(0)
	const streamed = answers.filter((one) => one.contentType.includes('event-stream'))
	expect(streamed, 'reads served as event streams spend the stream budget').toEqual([])
})
