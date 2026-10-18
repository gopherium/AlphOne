// SPDX-License-Identifier: AGPL-3.0-or-later

import { expect, test } from '@playwright/test'
import type { APIRequestContext, Page } from '@playwright/test'

import { createTask, graph } from '../graph'

const doneGroup = /^Done \(\d+\)$/

/** levelDay is the day the row geometry checks list their tasks on, kept clear of the days other tests fill. */
const levelDay = '2125-03-10'

/** orderDay is the day the priority order check lists its tasks on, kept clear of the days other tests fill. */
const orderDay = '2125-04-14'

/** timeDay is the day the time order check lists its tasks on, kept clear of the days other tests fill. */
const timeDay = '2125-05-19'

/** alignDay is the day the time column geometry check lists its tasks on, kept clear of the days other tests fill. */
const alignDay = '2125-06-16'

/** formDay is the day the time field check adds its task on, kept clear of the days other tests fill. */
const formDay = '2125-07-14'

/** halfDay is the day the half typed time check adds its tasks on, kept clear of the days other tests fill. */
const halfDay = '2125-08-18'

/** overdueDay is the past day the overdue column check leaves its tasks on, kept clear of the days other tests fill. */
const overdueDay = '2000-06-19'

/** badgeDay is the day the phone badge check lists its task on, kept clear of the days other tests fill. */
const badgeDay = '2125-09-22'

/** baselineDay is the day the phone time baseline check lists its tasks on, kept clear of the days other tests fill. */
const baselineDay = '2125-10-20'

/** ListedRow is a task row as the time checks read it, its title beside the text of its time column. */
type ListedRow = { title: string | null; time: string }

/** TimedPair names a task due at a time and a task with no time. */
type TimedPair = { timed: string; untimed: string }

/**
 * Reads the titles and times of the open rows of the day on screen, in the order they are shown.
 * @param page - The page showing a day of tasks.
 * @returns Each open row's title and the text of its time column.
 */
async function listedRows(page: Page): Promise<ListedRow[]> {
	return page
		.getByRole('list', { name: 'Open tasks', exact: true })
		.getByRole('listitem')
		.evaluateAll((rows) =>
			rows.map((row) => ({
				title: row.getAttribute('aria-label'),
				time: row.querySelector('.alphone-tasks__time')?.textContent ?? '',
			})),
		)
}

/**
 * Measures where the time column and the title of a row start and how wide the time column is.
 * @param page - The page showing the row.
 * @param title - The title of the task.
 * @returns The left edge and width of the time column and the left edge of the title.
 */
async function timeColumn(page: Page, title: string) {
	const row = page.getByRole('listitem', { name: title, exact: true })
	await expect(row).toBeVisible()
	return row.evaluate((node) => {
		const time = node.querySelector('.alphone-tasks__time')?.getBoundingClientRect()
		const cell = node.querySelector('.alphone-tasks__title')?.getBoundingClientRect()
		return { left: time?.left ?? Number.NaN, width: time?.width ?? 0, titleLeft: cell?.left ?? Number.NaN }
	})
}

/**
 * Checks that a timed and an untimed row draw one time column alike and start their titles level after it.
 * @param page - The page showing both rows.
 * @param pair - The titles of the timed and the untimed task.
 * @param width - The viewport width the rows are drawn at.
 */
async function expectLevelTimes(page: Page, pair: TimedPair, width: number) {
	const withTime = await timeColumn(page, pair.timed)
	const withoutTime = await timeColumn(page, pair.untimed)

	expect(withTime.width, `the time column is drawn at ${width}`).toBeGreaterThan(0)
	expect(withoutTime.width, `the blank column is as wide at ${width}`).toBeCloseTo(withTime.width, 0)
	expect(withoutTime.left, `the blank column starts level at ${width}`).toBeCloseTo(withTime.left, 0)
	expect(withoutTime.titleLeft, `the titles start level at ${width}`).toBeCloseTo(withTime.titleLeft, 0)
	expect(withTime.titleLeft, `the title starts after the time at ${width}`).toBeGreaterThan(
		withTime.left + withTime.width,
	)
}

/**
 * Measures where the time and the first title line of a row end, one font at one size, so equal ends share a baseline.
 * @param page - The page showing the row.
 * @param title - The title of the task.
 * @returns The bottom of the time text, the bottom of the first title line, and how many lines the title cell spans.
 */
async function firstLineBaselines(page: Page, title: string) {
	const row = page.getByRole('listitem', { name: title, exact: true })
	const lines = await row
		.getByRole('link', { name: title, exact: true })
		.evaluate((link) => [...link.getClientRects()].map((line) => line.bottom))
	const time = await row.locator('time.alphone-tasks__time').evaluate((clock) => {
		const range = document.createRange()
		range.selectNodeContents(clock)
		return range.getBoundingClientRect().bottom
	})
	const cellLines = await row
		.locator('.alphone-tasks__title')
		.evaluate((cell) =>
			Math.round(cell.getBoundingClientRect().height / Number.parseFloat(getComputedStyle(cell).lineHeight)),
		)
	return { time, title: lines[0], cellLines }
}

/**
 * Reads the due date label of a row and whether it follows the title, beside it or on a line below it.
 * @param page - The page showing the row.
 * @param title - The title of the task.
 * @returns The text of the due date label and whether it comes after the title.
 */
async function dueLabel(page: Page, title: string) {
	return page.getByRole('listitem', { name: title, exact: true }).evaluate((node) => {
		const due = node.querySelector('.alphone-tasks__due')
		const cell = node.querySelector('.alphone-tasks__title')?.getBoundingClientRect()
		const box = due?.getBoundingClientRect()
		if (!box || !cell) {
			return { text: due?.textContent ?? null, after: false }
		}
		return { text: due?.textContent ?? null, after: box.left >= cell.right || box.top >= cell.bottom }
	})
}

/**
 * Records the bodies of the task writes a page sends.
 * @param page - The page to listen on.
 * @returns The list the bodies of CreateTask and UpdateTask requests are added to as they leave.
 */
function taskWrites(page: Page): string[] {
	const writes: string[] = []
	page.on('request', (sent) => {
		const body = sent.postData() ?? ''
		if (/"operationName":"(CreateTask|UpdateTask)"/.test(body)) {
			writes.push(body)
		}
	})
	return writes
}

/**
 * Leaves the Time field half typed, its hour set and its minutes empty.
 * @param page - The page showing the Time field.
 * @param from - The time the field holds first, empty for a blank field.
 */
async function halfTypeTime(page: Page, from: string) {
	const time = page.getByLabel('Time', { exact: true })
	await time.click()
	if (from === '') {
		await page.keyboard.type('02')
	} else {
		await page.keyboard.press('ArrowRight')
		await page.keyboard.press('Backspace')
	}
	await expect.poll(() => time.evaluate((field: HTMLInputElement) => field.validity.badInput)).toBe(true)
	await expect(time).toHaveValue('')
}

/** LinkedPair names a task linked to a contact and a task with no contact, titled alike. */
type LinkedPair = { linked: string; unlinked: string }

/**
 * Adds a task to the day on screen through the quick add field.
 * @param page - The page showing a day of tasks.
 * @param title - The title of the task to add.
 */
async function quickAdd(page: Page, title: string) {
	await page.getByRole('textbox', { name: 'Task title' }).fill(title)
	await page.getByRole('button', { name: 'Add task' }).click()
	await expect(page.getByRole('listitem', { name: title })).toBeVisible()
}

/**
 * Creates two tasks on the level check day with titles of the same width, the first linked to a new contact.
 * @param request - The request context carrying the credential.
 * @param title - The words both titles start with.
 * @returns The titles of the linked and the unlinked task.
 */
async function seedLinkedPair(request: APIRequestContext, title: string): Promise<LinkedPair> {
	const stamp = Date.now()
	const created = await graph<{ createContact: { id: string } }>(
		request,
		'mutation($name: String!) { createContact(name: $name) { id } }',
		{ name: `Maria Perez ${stamp}` },
	)
	const pair = { linked: `${title} ${stamp}1`, unlinked: `${title} ${stamp}2` }
	await createTask(request, { title: pair.linked, dueOn: levelDay, contactId: created.createContact.id })
	await createTask(request, { title: pair.unlinked, dueOn: levelDay })
	return pair
}

/**
 * Measures a task row inside its borders, its title top from that inner top, and the lines its title wraps to.
 * @param page - The page showing the row.
 * @param title - The title of the task.
 * @returns The inner row height, the title top from the inner row top, and the title line count.
 */
async function rowGeometry(page: Page, title: string) {
	const row = page.getByRole('listitem', { name: title, exact: true })
	await expect(row).toBeVisible()
	const inner = await row.evaluate((node) => {
		const box = node.getBoundingClientRect()
		const style = getComputedStyle(node)
		const top = box.top + Number.parseFloat(style.borderTopWidth)
		return { top, height: box.bottom - Number.parseFloat(style.borderBottomWidth) - top }
	})
	const lines = await row
		.getByRole('link', { name: title, exact: true })
		.evaluate((link) => [...link.getClientRects()].map((line) => line.top))
	return { height: inner.height, titleTop: lines[0] - inner.top, lines: lines.length }
}

/**
 * Reads the colour a design token resolves to on the page.
 * @param page - The page carrying the design tokens.
 * @param token - The name of the colour token.
 * @returns The computed colour.
 */
async function tokenColor(page: Page, token: string): Promise<string> {
	return page.evaluate((name) => {
		const probe = document.createElement('span')
		probe.style.color = `var(${name})`
		document.body.append(probe)
		const color = getComputedStyle(probe).color
		probe.remove()
		return color
	}, token)
}

/**
 * Measures where the contact link of a linked row sits against its title.
 * @param page - The page showing the row.
 * @param title - The title of the linked task.
 * @returns Whether the link sits below the title, its height, its icon offset from the title and name from icon.
 */
async function contactPlacement(page: Page, title: string) {
	const row = page.getByRole('listitem', { name: title, exact: true })
	const contact = row.getByRole('link', { name: /^Maria Perez / })
	await expect(contact).toBeVisible()
	const lines = await row
		.getByRole('link', { name: title, exact: true })
		.evaluate((link) => [...link.getClientRects()].map((line) => ({ left: line.left, bottom: line.bottom })))
	const box = await contact.evaluate((link) => {
		const rect = link.getBoundingClientRect()
		const style = getComputedStyle(link)
		const icon = link.querySelector('svg') as SVGSVGElement
		const drawn = icon.getBoundingClientRect()
		const room = getComputedStyle(icon)
		const name = link.querySelector('.alphone-tasks__contact-name') as HTMLElement
		return {
			middle: rect.top + rect.height / 2,
			outer: rect.height + Number.parseFloat(style.marginTop) + Number.parseFloat(style.marginBottom),
			icon: drawn.left - Number.parseFloat(room.marginInlineStart),
			name: name.getBoundingClientRect().left,
			afterIcon: drawn.right + Number.parseFloat(room.marginInlineEnd) + Number.parseFloat(style.columnGap),
		}
	})
	return {
		ownLine: box.middle > (lines.at(-1)?.bottom ?? 0),
		outer: box.outer,
		iconInset: box.icon - (lines[0]?.left ?? 0),
		nameShift: box.name - box.afterIcon,
	}
}

/**
 * Checks that the linked row keeps its title level with the unlinked row and grows by the contact line alone.
 * @param page - The page showing both rows.
 * @param pair - The titles of the linked and the unlinked task.
 * @param lines - How many lines each title wraps to.
 * @returns Where the contact link sits.
 */
async function expectLevelRows(page: Page, pair: LinkedPair, lines: number) {
	const placement = await contactPlacement(page, pair.linked)
	const linked = await rowGeometry(page, pair.linked)
	const unlinked = await rowGeometry(page, pair.unlinked)
	const ownLine = placement.ownLine ? placement.outer : 0

	expect(linked.lines, 'the linked title wraps as asked').toBe(lines)
	expect(unlinked.lines, 'the unlinked title wraps as asked').toBe(lines)
	expect(linked.height, 'the contact name grows the row past its own line').toBeCloseTo(unlinked.height + ownLine, 0)
	expect(linked.titleTop, 'the contact name moves the title').toBeCloseTo(unlinked.titleTop, 0)
	expect(placement.nameShift, 'the contact name drifts from its icon').toBeCloseTo(0, 0)
	return placement
}

test('adds, completes, and reopens a task', async ({ page }) => {
	const title = `Call the supplier ${Date.now()}`

	await page.goto('/')
	await expect(page.getByRole('heading', { name: 'Tasks' })).toBeVisible()
	await quickAdd(page, title)

	const openTasks = page.getByRole('list', { name: 'Open tasks' })
	await expect(openTasks.getByRole('link', { name: title })).toBeVisible()

	await page.getByRole('listitem', { name: title }).getByRole('checkbox', { name: 'Complete' }).click()

	await expect(openTasks.getByRole('link', { name: title })).toBeHidden()
	await page.getByRole('button', { name: doneGroup }).click()
	const doneTasks = page.getByRole('list', { name: 'Done tasks' })
	await expect(doneTasks.getByRole('link', { name: title })).toBeVisible()

	await page.getByRole('listitem', { name: title }).getByRole('checkbox', { name: 'Reopen' }).click()

	await expect(openTasks.getByRole('link', { name: title })).toBeVisible()
})

test('pushes a task to the next day', async ({ page }) => {
	const title = `Send the quote ${Date.now()}`

	await page.goto('/')
	await quickAdd(page, title)

	await page
		.getByRole('listitem', { name: title })
		.getByRole('button', { name: 'Postpone' })
		.click()

	await expect(page.getByRole('listitem', { name: title })).toBeHidden()

	await page.getByRole('button', { name: 'Next day' }).click()

	await expect(page.getByRole('listitem', { name: title })).toBeVisible()
})

test('carries work left over from an earlier day into today', async ({ page }) => {
	const title = `Chase the invoice ${Date.now()}`

	await page.goto('/')
	await page.getByRole('button', { name: 'Previous day' }).click()
	await quickAdd(page, title)

	await page.getByRole('button', { name: 'Today' }).click()

	const overdue = page.getByRole('list', { name: 'Overdue tasks' })
	await expect(overdue.getByRole('link', { name: title })).toBeVisible()

	await page
		.getByRole('listitem', { name: title })
		.getByRole('button', { name: 'Postpone' })
		.click()

	await expect(page.getByRole('listitem', { name: title })).toBeHidden()

	await page.getByRole('button', { name: 'Next day' }).click()

	await expect(page.getByRole('listitem', { name: title })).toBeVisible()
})

test('adds a task with a due date and a priority from the tasks screen', async ({ page }) => {
	const title = `Order more boxes ${Date.now()}`

	await page.goto('/')
	await expect(page.getByRole('heading', { name: 'Tasks', exact: true })).toBeVisible()
	const browserDay = await page.evaluate(() => {
		const at = new Date()
		const today = at.toLocaleDateString('en-CA')
		at.setDate(at.getDate() + 1)
		return { today, tomorrow: at.toLocaleDateString('en-CA') }
	})

	await page.getByRole('textbox', { name: 'Task title', exact: true }).fill(title)
	await page.getByLabel('Due date', { exact: true }).fill(browserDay.tomorrow)
	await page.getByLabel('Priority', { exact: true }).click()
	await page.getByRole('option', { name: 'High', exact: true }).click()
	await page.getByRole('button', { name: 'Add task', exact: true }).click()

	const toasts = page.locator('.godmin-toasts')
	const added = `Task added for ${browserDay.tomorrow.split('-').reverse().join('/')}.`
	await expect(toasts.getByText(added, { exact: true })).toBeVisible()
	await expect(page.getByRole('textbox', { name: 'Task title', exact: true })).toHaveValue('')
	await expect(page.getByRole('textbox', { name: 'Task title', exact: true })).toBeFocused()
	await expect(page.getByLabel('Due date', { exact: true })).toHaveValue(browserDay.today)
	await expect(page.getByLabel('Priority', { exact: true })).toHaveText('Normal')
	await expect(page.getByRole('listitem', { name: title, exact: true })).toHaveCount(0)

	await toasts.getByRole('button', { name: 'View', exact: true }).click()

	const openTasks = page.getByRole('list', { name: 'Open tasks', exact: true })
	const row = openTasks.getByRole('listitem', { name: title, exact: true })
	await expect(row).toBeVisible()
	await expect(row.getByText('High', { exact: true })).toBeVisible()
})

test('lists a high task above the normal work of its day', async ({ page, request }) => {
	const stamp = Date.now()
	const normal = `Send the quote ${stamp}`
	const raised = `Approve the pricing ${stamp}`
	await createTask(request, { title: normal, dueOn: orderDay })
	await createTask(request, { title: raised, dueOn: orderDay, priority: 1 })

	await page.goto(`/tasks?date=${orderDay}`)

	const openTasks = page.getByRole('list', { name: 'Open tasks', exact: true })
	await expect(openTasks.getByRole('listitem', { name: normal, exact: true })).toBeVisible()
	const titles = await openTasks
		.getByRole('listitem')
		.evaluateAll((rows) => rows.map((row) => row.getAttribute('aria-label')))
	expect(titles.indexOf(raised)).toBeGreaterThanOrEqual(0)
	expect(titles.indexOf(raised)).toBeLessThan(titles.indexOf(normal))
})

test('lists the timed work of a day by priority, then by time, each time shown before its title', async ({
	page,
	request,
}) => {
	const stamp = Date.now()
	const late = `Call the supplier ${stamp}`
	const early = `Approve the pricing ${stamp}`
	const normal = `Send the quote ${stamp}`
	await createTask(request, { title: late, dueOn: timeDay, dueTime: '11:00', priority: 1 })
	await createTask(request, { title: early, dueOn: timeDay, dueTime: '09:00', priority: 1 })
	await createTask(request, { title: normal, dueOn: timeDay, dueTime: '09:00' })

	await page.goto(`/tasks?date=${timeDay}`)

	await expect(page.getByRole('listitem', { name: normal, exact: true })).toBeVisible()
	const ours = (await listedRows(page)).filter((row) => row.title?.endsWith(` ${stamp}`))
	expect(ours).toEqual([
		{ title: early, time: '09:00' },
		{ title: late, time: '11:00' },
		{ title: normal, time: '09:00' },
	])
})

test('lines the titles up behind one time column on timed and untimed rows, on a desktop and on a phone', async ({
	page,
	request,
}) => {
	const stamp = Date.now()
	const pair = { timed: `Confirm the delivery window ${stamp}`, untimed: `Check the stock count ${stamp}` }
	await createTask(request, { title: pair.timed, dueOn: alignDay, dueTime: '08:45' })
	await createTask(request, { title: pair.untimed, dueOn: alignDay })

	for (const size of [
		{ width: 1280, height: 800 },
		{ width: 390, height: 844 },
	]) {
		await page.setViewportSize(size)
		await page.goto(`/tasks?date=${alignDay}`)
		await expectLevelTimes(page, pair, size.width)
	}
})

test('lines the overdue titles up behind one time column, due dates after them, on a desktop and a phone', async ({
	page,
	request,
}) => {
	const stamp = Date.now()
	const pair = { timed: `Chase the unpaid invoice ${stamp}`, untimed: `Renew the shop domain ${stamp}` }
	const timed = await createTask(request, { title: pair.timed, dueOn: overdueDay, dueTime: '08:45' })
	const untimed = await createTask(request, { title: pair.untimed, dueOn: overdueDay })

	try {
		for (const size of [
			{ width: 1280, height: 800 },
			{ width: 390, height: 844 },
		]) {
			await page.setViewportSize(size)
			await page.goto('/tasks')
			const overdue = page.getByRole('list', { name: 'Overdue tasks', exact: true })
			await expect(overdue.getByRole('listitem', { name: pair.untimed, exact: true })).toBeVisible()
			await expect(overdue.getByRole('listitem', { name: pair.timed, exact: true })).toBeVisible()
			await expectLevelTimes(page, pair, size.width)
			for (const title of [pair.timed, pair.untimed]) {
				expect(await dueLabel(page, title), `the due date follows the title at ${size.width}`).toEqual({
					text: 'Due 19/06/2000',
					after: true,
				})
			}
		}
	} finally {
		for (const task of [timed, untimed]) {
			await graph(
				request,
				'mutation($id: UUID!, $input: UpdateTaskInput!) { updateTask(id: $id, input: $input) { id } }',
				{ id: task.id, input: { status: 'done' } },
			)
		}
	}
})

test('keeps the High badge of a timed task on its title line on a phone in English', async ({ page, request }) => {
	const title = `Approve the pricing ${Date.now()}`
	await createTask(request, { title, dueOn: badgeDay, dueTime: '09:30', priority: 1 })

	await page.setViewportSize({ width: 390, height: 844 })
	await page.goto(`/tasks?date=${badgeDay}`)

	const row = page.getByRole('listitem', { name: title, exact: true })
	const badge = row.getByText('High', { exact: true })
	await expect(badge).toBeVisible()
	const cell = await row.locator('.alphone-tasks__title').evaluate((node) => {
		const box = node.getBoundingClientRect()
		return { top: box.top, bottom: box.bottom }
	})
	const middle = await badge.evaluate((node) => {
		const box = node.getBoundingClientRect()
		return box.top + box.height / 2
	})

	expect(middle, 'the badge drops below the title line').toBeGreaterThan(cell.top)
	expect(middle, 'the badge drops below the title line').toBeLessThan(cell.bottom)
})

test('sets the time level with the first line of a wrapped title and of a title with its contact, on a phone', async ({
	page,
	request,
}) => {
	const stamp = Date.now()
	const contact = await graph<{ createContact: { id: string } }>(
		request,
		'mutation($name: String!) { createContact(name: $name) { id } }',
		{ name: `Maria Perez ${stamp}` },
	)
	const wrapped = `Confirm the delivery window with the warehouse team before the weekend ${stamp}`
	const linked = `Call the supplier back ${stamp}`
	const contactId = contact.createContact.id
	await createTask(request, { title: wrapped, dueOn: baselineDay, dueTime: '08:45' })
	await createTask(request, { title: linked, dueOn: baselineDay, dueTime: '11:30', contactId })

	await page.setViewportSize({ width: 390, height: 844 })
	await page.goto(`/tasks?date=${baselineDay}`)
	await expect(page.getByRole('listitem', { name: linked, exact: true })).toBeVisible()

	for (const title of [wrapped, linked]) {
		const level = await firstLineBaselines(page, title)
		const offset = Math.abs(level.time - level.title)
		expect(level.cellLines, `the title cell of ${title} spans more than one line`).toBeGreaterThan(1)
		expect(offset, `the time of ${title} sits off its first title line`).toBeLessThanOrEqual(2)
	}
})

test('holds a half typed time back from a save and from the quick add, sending nothing until it is whole', async ({
	page,
	request,
}) => {
	const stamp = Date.now()
	const saved = `Pack the samples ${stamp}`
	const added = `Label the samples ${stamp}`
	const created = await createTask(request, { title: saved, dueOn: halfDay, dueTime: '09:30' })
	const writes = taskWrites(page)

	await page.goto(`/tasks/${created.id}`)
	await expect(page.getByLabel('Time', { exact: true })).toHaveValue('09:30')
	await halfTypeTime(page, '09:30')
	await page.getByRole('button', { name: 'Save', exact: true }).click()
	await page.getByLabel('Time', { exact: true }).fill('09:45')
	await page.getByRole('button', { name: 'Save', exact: true }).click()
	await expect(page.locator('.godmin-toasts').getByText('Task saved.', { exact: true })).toBeVisible()
	expect(writes.map((body) => JSON.parse(body).variables.input.dueTime)).toEqual(['09:45'])

	await page.goto(`/tasks?date=${halfDay}`)
	await page.getByRole('textbox', { name: 'Task title', exact: true }).fill(added)
	await halfTypeTime(page, '')
	await page.getByRole('button', { name: 'Add task', exact: true }).click()
	await page.getByRole('textbox', { name: 'Task title', exact: true }).press('Enter')
	await page.getByLabel('Time', { exact: true }).fill('14:30')
	await page.getByRole('button', { name: 'Add task', exact: true }).click()
	await expect(page.getByRole('listitem', { name: added, exact: true }).locator('time')).toHaveText('14:30')
	expect(writes.map((body) => JSON.parse(body).variables.input.dueTime)).toEqual(['09:45', '14:30'])
})

test('adds a task at a time from the tasks screen, shows it on the row and clears it from the detail', async ({
	page,
}) => {
	const title = `Book the courier ${Date.now()}`

	await page.goto(`/tasks?date=${formDay}`)
	await page.getByRole('textbox', { name: 'Task title', exact: true }).fill(title)
	await page.getByLabel('Time', { exact: true }).fill('14:30')
	await page.getByRole('button', { name: 'Add task', exact: true }).click()

	const row = page.getByRole('listitem', { name: title, exact: true })
	await expect(row.locator('time.alphone-tasks__time')).toHaveText('14:30')
	await expect(row.locator('time.alphone-tasks__time')).toHaveAttribute('datetime', '14:30')
	await expect(page.getByLabel('Time', { exact: true })).toHaveValue('')

	await row.getByRole('link', { name: title, exact: true }).click()
	await expect(page.getByRole('heading', { name: title })).toBeVisible()
	await expect(page.getByLabel('Time', { exact: true })).toHaveValue('14:30')
	await page.getByLabel('Time', { exact: true }).fill('')
	await page.getByRole('button', { name: 'Save', exact: true }).click()
	await expect(page.locator('.godmin-toasts').getByText('Task saved.', { exact: true })).toBeVisible()

	await page.goto(`/tasks?date=${formDay}`)
	await expect(row).toBeVisible()
	await expect(row.locator('.alphone-tasks__time')).toHaveText('')
	await expect(row.locator('time')).toHaveCount(0)
})

test('opens the contact of a task from its name on the row', async ({ page, request }) => {
	const stamp = Date.now()
	const contact = `Maria Perez ${stamp}`
	const title = `Call ${contact} back`
	const created = await graph<{ createContact: { id: string } }>(
		request,
		'mutation($name: String!) { createContact(name: $name) { id } }',
		{ name: contact },
	)
	const browserToday = await page.evaluate(() => new Date().toLocaleDateString('en-CA'))
	await createTask(request, {
		title,
		dueOn: browserToday,
		contactId: created.createContact.id,
	})

	await page.goto('/tasks')
	const row = page.getByRole('listitem', { name: title, exact: true })
	await row.getByRole('link', { name: contact, exact: true }).click()

	await expect(page).toHaveURL(new RegExp(`/contacts/${created.createContact.id}$`))
	await expect(page.getByRole('heading', { level: 1, name: contact, exact: true })).toBeVisible()
})

test('keeps a row with the contact name as tall as a row without, titles level, on a desktop', async ({
	page,
	request,
}) => {
	const titles = await seedLinkedPair(request, 'Confirm the delivery window')

	await page.setViewportSize({ width: 1280, height: 800 })
	await page.goto(`/tasks?date=${levelDay}`)

	expect((await expectLevelRows(page, titles, 1)).ownLine, 'the contact name leaves the title line').toBe(false)
})

test('draws the contact name in the weak text colour, darker while pointed at, on a desktop', async ({
	page,
	request,
}) => {
	const titles = await seedLinkedPair(request, 'Check the stock count')

	await page.setViewportSize({ width: 1280, height: 800 })
	await page.goto(`/tasks?date=${levelDay}`)

	const link = page
		.getByRole('listitem', { name: titles.linked, exact: true })
		.getByRole('link', { name: /^Maria Perez / })
	await expect(link).toBeVisible()
	await expect(link).toHaveCSS('color', await tokenColor(page, '--wpds-color-foreground-content-neutral-weak'))
	await link.hover()
	await expect(link).toHaveCSS('color', await tokenColor(page, '--wpds-color-foreground-interactive-neutral-active'))
})

test('puts the contact name on its own line under a wrapped title, flush with it, titles level, on a phone', async ({
	page,
	request,
}) => {
	const titles = await seedLinkedPair(request, 'Confirm the delivery window with the warehouse team')

	await page.setViewportSize({ width: 390, height: 844 })
	await page.goto(`/tasks?date=${levelDay}`)

	const placement = await expectLevelRows(page, titles, 2)
	expect(placement.ownLine, 'the contact name shares a title line').toBe(true)
	expect(placement.iconInset, 'the person icon is indented from the title').toBeCloseTo(0, 0)
})

test('strikes through a done title but not its contact name, on a phone', async ({ page, request }) => {
	const stamp = Date.now()
	const title = `Approve the pricing ${stamp}`
	const created = await graph<{ createContact: { id: string } }>(
		request,
		'mutation($name: String!) { createContact(name: $name) { id } }',
		{ name: `Maria Perez ${stamp}` },
	)
	const task = await createTask(request, { title, dueOn: levelDay, contactId: created.createContact.id })
	await graph(
		request,
		'mutation($id: UUID!, $input: UpdateTaskInput!) { updateTask(id: $id, input: $input) { id } }',
		{ id: task.id, input: { status: 'done' } },
	)

	await page.setViewportSize({ width: 390, height: 844 })
	await page.goto(`/tasks?date=${levelDay}`)
	await page.getByRole('button', { name: doneGroup }).click()

	const row = page.getByRole('list', { name: 'Done tasks' }).getByRole('listitem', { name: title, exact: true })
	await expect(row.getByRole('link', { name: title, exact: true })).toHaveCSS('text-decoration-line', 'line-through')
	await expect(row.locator('.alphone-tasks__title')).toHaveCSS('text-decoration-line', 'none')
})

test('cuts a long contact name short on one line inside the title, on a phone', async ({ page, request }) => {
	const stamp = Date.now()
	const contact = `Maria Perez from the northern warehouse returns team ${stamp}`
	const title = `Book the returns pickup ${stamp}`
	const created = await graph<{ createContact: { id: string } }>(
		request,
		'mutation($name: String!) { createContact(name: $name) { id } }',
		{ name: contact },
	)
	await createTask(request, { title, dueOn: levelDay, contactId: created.createContact.id })

	await page.setViewportSize({ width: 390, height: 844 })
	await page.goto(`/tasks?date=${levelDay}`)

	const row = page.getByRole('listitem', { name: title, exact: true })
	const link = row.getByRole('link', { name: contact, exact: true })
	await expect(link).toBeVisible()
	const fit = await link.evaluate((node) => {
		const name = node.querySelector('.alphone-tasks__contact-name')
		const icon = node.querySelector('svg')
		const cell = node.closest('.alphone-tasks__title')
		return {
			height: node.getBoundingClientRect().height,
			minHeight: Number.parseFloat(getComputedStyle(node).minHeight),
			cut: name !== null && name.scrollWidth > name.clientWidth,
			overflow: node.getBoundingClientRect().right - (cell?.getBoundingClientRect().right ?? 0),
			icon: icon?.getBoundingClientRect().width ?? 0,
			iconSize: Number(icon?.getAttribute('width')),
		}
	})

	expect(fit.cut, 'the long name is cut short').toBe(true)
	expect(fit.height, 'the long name wraps').toBeCloseTo(fit.minHeight, 0)
	expect(fit.overflow, 'the long name runs past the title').toBeLessThanOrEqual(0)
	expect(fit.icon, 'the long name squeezes the person icon').toBeCloseTo(fit.iconSize, 0)
})

test('opens a task from the day list', async ({ page }) => {
	const title = `Approve the pricing ${Date.now()}`

	await page.goto('/')
	await quickAdd(page, title)

	await page.getByRole('link', { name: title }).click()

	await expect(page.getByRole('heading', { name: title })).toBeVisible()
	await expect(page.getByLabel('Title')).toHaveValue(title)

	await page.getByLabel('Title').fill(`${title} today`)
	await page.getByRole('button', { name: 'Save' }).click()

	await expect(page.getByRole('heading', { name: `${title} today` })).toBeVisible()
})

test('adds a task from a contact and links it back', async ({ page }) => {
	const stamp = Date.now()
	const contact = `Customer ${stamp}`
	const title = `Call ${contact}`

	await page.goto('/')
	await page.getByRole('link', { name: 'Contacts' }).click()
	await page.getByRole('link', { name: 'New contact' }).click()
	await page.getByLabel('Name').fill(contact)
	await page.getByRole('button', { name: 'Create contact' }).click()
	await expect(page.getByRole('heading', { name: contact })).toBeVisible()

	await page.getByRole('textbox', { name: 'Task title' }).fill(title)
	await page.getByRole('button', { name: 'Add task' }).click()

	const contactTasks = page.getByRole('list', { name: 'Contact tasks' })
	await expect(contactTasks.getByRole('link', { name: title })).toBeVisible()

	await contactTasks.getByRole('link', { name: title }).click()

	await expect(page.getByRole('heading', { name: title })).toBeVisible()
	await expect(page.getByRole('link', { name: contact })).toBeVisible()
})

test('adds a task from a contact on a chosen day', async ({ page }) => {
	const stamp = Date.now()
	const contact = `Customer ${stamp}`
	const title = `Follow up with ${contact}`
	const chosen = new Date()
	chosen.setDate(chosen.getDate() + 3)
	const due = chosen.toLocaleDateString('en-CA')
	const label = `Due ${due.split('-').reverse().join('/')}`

	await page.goto('/')
	await page.getByRole('link', { name: 'Contacts' }).click()
	await page.getByRole('link', { name: 'New contact' }).click()
	await page.getByLabel('Name').fill(contact)
	await page.getByRole('button', { name: 'Create contact' }).click()
	await expect(page.getByRole('heading', { name: contact })).toBeVisible()

	await page.getByRole('textbox', { name: 'Task title' }).fill(title)
	await page.getByLabel('Due date', { exact: true }).fill(due)
	await page.getByRole('button', { name: 'Add task' }).click()

	const row = page.getByRole('list', { name: 'Contact tasks' }).getByRole('listitem', { name: title })
	await expect(row).toContainText(label)
	const browserToday = await page.evaluate(() => new Date().toLocaleDateString('en-CA'))
	await expect(page.getByLabel('Due date', { exact: true })).toHaveValue(browserToday)

	await row.getByRole('link', { name: title }).click()

	await expect(page.getByRole('heading', { name: title })).toBeVisible()
	await expect(page.getByLabel('Due date', { exact: true })).toHaveValue(due)
})
