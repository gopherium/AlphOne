// SPDX-License-Identifier: AGPL-3.0-or-later

import type { Page, Response } from '@playwright/test'

/**
 * Waits for the page to establish the named graph subscription.
 * @param page - The page opening the subscription.
 * @param field - The subscription root field the document selects.
 * @returns The subscription's streaming response.
 */
export function subscribed(page: Page, field: string): Promise<Response> {
	return page.waitForResponse(
		(response) =>
			response.url().includes('/api/graphql') &&
			(response.request().postData() ?? '').includes(field),
	)
}
