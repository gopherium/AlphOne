// SPDX-License-Identifier: AGPL-3.0-or-later

import { expect, test } from '@playwright/test'
import type { APIRequestContext, Locator, Page } from '@playwright/test'

import { ingest, knowSender, mailboxAddress } from '../../../test/e2e/ingest'

/** sealedArmor is the body line of a PGP message that no key opens. */
const sealedArmor = 'hF4DAAAAAAAAAAASAQdAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA'

/** sealedBody holds the lines of a PGP message that no key opens, which the inbox marks with a badge. */
const sealedBody = ['-----BEGIN PGP MESSAGE-----', '', sealedArmor, '=AAAA', '-----END PGP MESSAGE-----']

/** plainBody holds the one line of a mail anyone reads, longer than any preview the inbox draws. */
const plainBody = ['Could the lamps and the shade reach the hall before the opening on Saturday morning?']

/** longSubject is a subject far longer than any width the inbox gives it. */
const longSubject = 'Question about delivering the three wicker lamps and the matching shade to the new hall'

/** tabletAndSmallLaptop are the sizes where the inbox shows every column with the least room for them. */
const tabletAndSmallLaptop = [
	{ width: 1024, height: 768 },
	{ width: 783, height: 900 },
]

/** twoDigitMoment is a moment whose day of the month and hour each take two digits. */
const twoDigitMoment = '2026-11-23T11:30:00Z'

/** InboxRead is the part of the inbox read that holds the date of each thread. */
type InboxRead = { data: { emailThreads: { items: { lastMessageAt: string }[] } } }

/**
 * Returns the start of a minute, counted back from the current one.
 * @param minutesBefore - How many minutes before the current one.
 * @returns The moment.
 */
function wholeMinute(minutesBefore: number): Date {
	return new Date((Math.floor(Date.now() / 60_000) - minutesBefore) * 60_000)
}

/**
 * Returns a moment as an English list writes it in UTC.
 * @param at - The moment.
 * @returns The date and time, such as Nov 2, 2026 9:05 am.
 */
function listed(at: Date): string {
	const day = at.toLocaleDateString('en-US', { month: 'short', day: 'numeric', year: 'numeric', timeZone: 'UTC' })
	const hours = at.getUTCHours()
	const minutes = String(at.getUTCMinutes()).padStart(2, '0')
	return `${day} ${hours % 12 || 12}:${minutes} ${hours < 12 ? 'am' : 'pm'}`
}

/**
 * Answers each inbox read of a page with every thread dated at the same moment.
 * @param page - The page that shows the inbox.
 * @param at - The moment, as the graph writes it.
 */
async function pinInboxDates(page: Page, at: string): Promise<void> {
	await page.route('**/api/graphql', async (route) => {
		if (!route.request().postData()?.includes('EmailInbox')) {
			await route.continue()
			return
		}
		const response = await route.fetch()
		const json = (await response.json()) as InboxRead
		for (const thread of json.data.emailThreads.items) {
			thread.lastMessageAt = at
		}
		await route.fulfill({ response, json })
	})
}

/**
 * Returns the inbox row of the thread with the given subject.
 * @param page - The page showing the inbox.
 * @param subject - The thread's subject.
 * @returns The row.
 */
function threadRow(page: Page, subject: string): Locator {
	return page.getByRole('region', { name: 'Inbox' }).getByRole('row').filter({ hasText: subject })
}

/**
 * Returns the cell of a row under the column whose heading starts with the given words.
 * @param page - The page showing the inbox.
 * @param row - The row.
 * @param heading - The start of the column heading.
 * @returns The cell.
 */
async function cellUnder(page: Page, row: Locator, heading: string): Promise<Locator> {
	const headings = await page.getByRole('region', { name: 'Inbox' }).getByRole('columnheader').allTextContents()
	const column = headings.findIndex((text) => text.startsWith(heading))
	expect(column, `the inbox holds a ${heading} column`).toBeGreaterThanOrEqual(0)
	return row.getByRole('cell').nth(column)
}

/**
 * Feeds one mail whose subject, sender name and address run far longer than the inbox has room for.
 * @param request - The Playwright request context carrying the credential.
 * @param body - The lines of the mail.
 * @returns The thread's subject.
 */
async function feedLongSubject(request: APIRequestContext, body: string[]): Promise<string> {
	const stamp = Date.now()
	const sender = {
		name: `Maria Perez de la Fuente y Rodriguez ${stamp}`,
		email: `maria.perez.de.la.fuente.y.rodriguez.${stamp}@example.com`,
	}
	const subject = `${longSubject} ${stamp}`
	await knowSender(request, sender)
	expect(
		ingest({
			from: sender,
			subject,
			id: `long-subject.${stamp}@example.com`,
			receivedAt: wholeMinute(0),
			body,
		}),
	).toBe('stored')
	return subject
}

/**
 * Returns how many pixels the inbox table runs past the end of the page canvas, none or less when it fits.
 * @param page - The page showing the inbox.
 * @returns The pixels past the canvas.
 */
function pastCanvas(page: Page): Promise<number> {
	return page
		.getByRole('region', { name: 'Inbox' })
		.locator('table')
		.evaluate((table) => {
			const canvas = table.closest('.godmin-layout__canvas')
			return Math.round(table.getBoundingClientRect().right - (canvas?.getBoundingClientRect().right ?? 0))
		})
}

/**
 * Returns how many pixels the narrowest subject of the Subject column falls short of its room beside its marks.
 * @param page - The page showing the inbox.
 * @returns The pixels short of 4em, or of its line or whole text when shorter, none when each subject keeps it.
 */
function subjectShortOfRoom(page: Page): Promise<number> {
	return page.getByRole('region', { name: 'Inbox' }).evaluate((region) => {
		const lines = [...region.querySelectorAll('.alphone-email-inbox__cell > .alphone-email-inbox__line')]
		return Math.max(
			0,
			...lines.map((line) => {
				const text = line.querySelector('.alphone-email-inbox__text') as HTMLElement
				const em = parseFloat(getComputedStyle(text).fontSize)
				const room = Math.min(4 * em, line.getBoundingClientRect().width, text.scrollWidth)
				return Math.round(room - text.getBoundingClientRect().width)
			}),
		)
	})
}

/**
 * Returns how many pixels the widest part of a subject line, its subject or a mark, runs past its column's inner edge.
 * @param page - The page showing the inbox.
 * @returns The pixels past the column, none when each part fits, leaving out a mark wider than the whole column.
 */
function linePastColumn(page: Page): Promise<number> {
	return page.getByRole('region', { name: 'Inbox' }).evaluate((region) => {
		const lines = [...region.querySelectorAll('.alphone-email-inbox__cell > .alphone-email-inbox__line')]
		return Math.max(
			0,
			...lines.flatMap((line) => {
				const column = line.closest('td') as HTMLElement
				const style = getComputedStyle(column)
				const box = column.getBoundingClientRect()
				const edge = box.right - parseFloat(style.paddingRight)
				const room = edge - box.left - parseFloat(style.paddingLeft)
				return [...line.children]
					.map((part) => part.getBoundingClientRect())
					.filter((part) => part.width <= room)
					.map((part) => Math.round(part.right - edge))
			}),
		)
	})
}

/**
 * Returns each mark of the Subject column that sits off its subject's line or past the column's inner edge.
 * @param page - The page showing the inbox.
 * @returns The marks out of place, named by subject and words or title, none when each sits beside its subject.
 */
function marksOffSubjectLine(page: Page): Promise<string[]> {
	return page.getByRole('region', { name: 'Inbox' }).evaluate((region) => {
		const lines = [...region.querySelectorAll('.alphone-email-inbox__cell > .alphone-email-inbox__line')]
		return lines.flatMap((line) => {
			const column = line.closest('td') as HTMLElement
			const edge = column.getBoundingClientRect().right - parseFloat(getComputedStyle(column).paddingRight)
			const text = line.querySelector('.alphone-email-inbox__text') as HTMLElement
			const subject = text.getBoundingClientRect()
			return [...line.children]
				.filter((part) => part !== text)
				.filter((mark) => {
					const box = mark.getBoundingClientRect()
					const middle = box.top + box.height / 2
					return middle < subject.top || middle > subject.bottom || box.right > edge
				})
				.map((mark) => `${text.title}: ${mark.textContent || mark.querySelector('[title]')?.getAttribute('title')}`)
		})
	})
}

/**
 * Returns how many pixels the page or a scrolling box of the inbox scrolls sideways, none when the list fits.
 * @param page - The page showing the inbox.
 * @returns The widest sideways scroll.
 */
function sidewaysScroll(page: Page): Promise<number> {
	return page.getByRole('region', { name: 'Inbox' }).evaluate((region) => {
		const scrolling = [region, ...region.querySelectorAll('*')].filter((box) =>
			['auto', 'scroll'].includes(getComputedStyle(box).overflowX),
		)
		const root = document.documentElement
		return Math.max(root.scrollWidth - root.clientWidth, ...scrolling.map((box) => box.scrollWidth - box.clientWidth))
	})
}

test('the Email row shows once a mail is fed, and the inbox lists its thread', async ({ page, request }) => {
	const stamp = Date.now()
	const sender = { name: `Maria Perez ${stamp}`, email: `maria.perez.${stamp}@example.com` }
	const subject = `Opening hours ${stamp}`
	const asked = `opening-hours.${stamp}@example.com`
	const repliedAt = wholeMinute(0)
	await knowSender(request, sender)
	expect(
		ingest({
			from: sender,
			subject,
			id: asked,
			receivedAt: wholeMinute(1),
			body: ['When does the shop open on Saturday?'],
		}),
	).toBe('stored')
	expect(
		ingest({
			from: sender,
			subject: `Re: ${subject}`,
			id: `opening-hours-again.${stamp}@example.com`,
			receivedAt: repliedAt,
			inReplyTo: asked,
			body: ['And does it open on Sunday too?'],
		}),
	).toBe('stored')

	await page.goto('/')
	await page.getByRole('region', { name: 'Navigation' }).getByRole('link', { name: 'Email', exact: true }).click()

	await expect(page.getByRole('heading', { level: 2, name: 'Email' })).toBeVisible()
	await expect(page.getByText(`Read the mail of ${mailboxAddress} in one place.`)).toBeVisible()
	const row = threadRow(page, subject)
	await expect(row).toHaveCount(1)
	await expect(row).toContainText(sender.name)
	await expect(row).toContainText(sender.email)
	await expect(row).toContainText('And does it open on Sunday too?')
	await expect(await cellUnder(page, row, 'Messages')).toHaveText('2')
	await expect(await cellUnder(page, row, 'Date')).toHaveText(listed(repliedAt))
})

test('an encrypted mail shows its badge and a fixed preview in the inbox', async ({ page, request }) => {
	const stamp = Date.now()
	const sender = { name: `Maria Perez ${stamp}`, email: `maria.perez.${stamp}@example.com` }
	const subject = `Payment details ${stamp}`
	await knowSender(request, sender)
	expect(
		ingest({
			from: sender,
			subject,
			id: `payment-details.${stamp}@example.com`,
			receivedAt: wholeMinute(0),
			body: sealedBody,
		}),
	).toBe('stored')

	await page.goto('/email')

	const row = threadRow(page, subject)
	await expect(row).toHaveCount(1)
	await expect(row.getByText('Encrypted', { exact: true })).toBeVisible()
	await expect(row.getByText('Encrypted message', { exact: true })).toBeVisible()
	await expect(row).not.toContainText('BEGIN PGP')
})

for (const size of tabletAndSmallLaptop) {
	test(`a long subject gives way so the inbox fits the canvas at ${size.width}px`, async ({ page, request }) => {
		const subject = await feedLongSubject(request, plainBody)
		const sealed = await feedLongSubject(request, sealedBody)
		await page.setViewportSize(size)

		await page.goto('/email')

		await expect(threadRow(page, subject)).toHaveCount(1)
		await expect(threadRow(page, sealed)).toHaveCount(1)
		await expect.poll(() => pastCanvas(page)).toBeLessThanOrEqual(0)
		const date = page.getByRole('region', { name: 'Inbox' }).getByRole('columnheader', { name: /Date/ })
		await expect(date).toBeInViewport({ ratio: 1 })
		expect(await subjectShortOfRoom(page)).toBe(0)
		expect(await linePastColumn(page)).toBe(0)
	})

	test(`a long sender leaves each subject its marks on its own line at ${size.width}px`, async ({ page, request }) => {
		const sealed = await feedLongSubject(request, sealedBody)
		await page.setViewportSize(size)
		await pinInboxDates(page, twoDigitMoment)

		await page.goto('/email')

		const row = threadRow(page, sealed)
		await expect(row).toHaveCount(1)
		await expect(await cellUnder(page, row, 'Date')).toHaveText(listed(new Date(twoDigitMoment)))
		await expect.poll(() => marksOffSubjectLine(page)).toEqual([])
	})
}

test.describe('on a 320px phone with a touch screen', () => {
	test.use({ viewport: { width: 320, height: 640 }, isMobile: true, hasTouch: true })

	test('a long subject never pushes the inbox sideways', async ({ page, request }) => {
		const subject = await feedLongSubject(request, plainBody)

		await page.goto('/email')

		await expect(threadRow(page, subject)).toHaveCount(1)
		await expect.poll(() => sidewaysScroll(page)).toBe(0)
	})
})
