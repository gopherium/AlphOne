// SPDX-License-Identifier: AGPL-3.0-or-later

import { GraphProvider, createGraphClient, useAdminSettings } from '@alphone/frontend-sdk'
import type { AdminSettingsRead } from '@alphone/frontend-sdk'
import { HttpResponse, adminSession, graphql, seedSession, server } from '@alphone/frontend-sdk/testing'
import { ConfirmDialog } from '@gopherium/godmin'
import { createAuthQueryClient } from '@gopherium/react-auth'
import type { User } from '@gopherium/react-auth'
import { QueryClientProvider } from '@tanstack/react-query'
import { act, fireEvent, render, screen, waitFor, within } from '@testing-library/react'
import { afterEach, expect, test, vi } from 'vitest'

import { AdminHost } from '../AdminHost'

/** The words godmin shows once a write ran past its time limit. */
const timedOutWords = 'Could not get a valid response from the server.'

/** The longest time limit in milliseconds godmin takes short of none. */
const longestLimit = 2 ** 31 - 1

afterEach(() => {
	vi.useRealTimers()
})

/**
 * Names how far the admin settings read got.
 * @param read - What the settings hook answered.
 * @returns Served, failed or waiting.
 */
function readState({ settings, failed }: AdminSettingsRead): string {
	if (settings !== undefined) {
		return 'served'
	}
	return failed ? 'failed' : 'waiting'
}

/** Shows how far the admin settings read got. */
function ReadState() {
	return <output aria-label="Settings read">{readState(useAdminSettings())}</output>
}

/**
 * Returns what shows how far the admin settings read got, behind the modal confirm.
 * @returns The output.
 */
function readProbe(): HTMLElement {
	return screen.getByRole('status', { name: 'Settings read', hidden: true })
}

/**
 * Asks to delete one contact, its write never answering, handing the signal of each write to the test.
 * @param props - What takes the signal a write was handed.
 * @returns The confirm.
 */
function WaitingDelete({ onSignal }: { onSignal: (signal: AbortSignal) => void }) {
	return (
		<ConfirmDialog
			title="Delete Grace Hopper?"
			confirmLabel="Delete"
			cancelLabel="Cancel"
			onConfirm={(signal) => {
				onSignal(signal)
				return new Promise(() => {})
			}}
			onClose={() => {}}
		/>
	)
}

/**
 * Renders the admin host around a waiting delete, signed in as the given account.
 * @param user - The signed-in account, or null for nobody.
 * @returns The signals the writes of the delete were handed, in order.
 */
function renderHost(user: (User & { role?: string }) | null): AbortSignal[] {
	const signals: AbortSignal[] = []
	const client = createAuthQueryClient({ queries: { retry: false, staleTime: Infinity } })
	seedSession(client, user)
	render(
		<QueryClientProvider client={client}>
			<GraphProvider graph={createGraphClient({ onSessionExpired: () => {} })}>
				<AdminHost>
					<ReadState />
					<WaitingDelete onSignal={(signal) => signals.push(signal)} />
				</AdminHost>
			</GraphProvider>
		</QueryClientProvider>,
	)
	return signals
}

/** Presses Delete in the confirm under a clock that runs only when the test runs it. */
async function pressDeleteOnAHeldClock() {
	const dialog = within(await screen.findByRole('alertdialog'))
	vi.useFakeTimers({ toFake: ['setTimeout', 'clearTimeout'] })
	fireEvent.click(dialog.getByRole('button', { name: 'Delete' }))
}

/**
 * Runs the held clock on by the given time.
 * @param milliseconds - How far the clock runs.
 */
async function runClock(milliseconds: number) {
	await act(async () => {
		await vi.advanceTimersByTimeAsync(milliseconds)
	})
}

/**
 * Checks that the one write the delete sent still runs, past every time limit godmin takes.
 * @param signals - The signals the writes of the delete were handed.
 */
async function expectNoTimeLimit(signals: AbortSignal[]) {
	await pressDeleteOnAHeldClock()
	await runClock(longestLimit + 1)

	expect(signals).toHaveLength(1)
	expect(signals[0]?.aborted).toBe(false)
	expect(screen.queryByText(timedOutWords)).not.toBeInTheDocument()
}

test('gives a busy dialog no time limit before anyone signs in', async () => {
	const signals = renderHost(null)

	expect(readProbe()).toHaveTextContent('waiting')
	await expectNoTimeLimit(signals)
})

test('gives a busy dialog no time limit while the admin settings still load', async () => {
	let asked!: () => void
	const reading = new Promise<void>((resolve) => {
		asked = resolve
	})
	server.use(
		graphql.query('AdminSettings', () => {
			asked()
			return new Promise(() => {})
		}),
	)
	const signals = renderHost(adminSession)
	await reading

	expect(readProbe()).toHaveTextContent('waiting')
	await expectNoTimeLimit(signals)
})

test('gives a busy dialog no time limit when the admin settings could not be read', async () => {
	server.use(
		graphql.query('AdminSettings', () =>
			HttpResponse.json({ data: null, errors: [{ message: 'internal error' }] }),
		),
	)
	const signals = renderHost(adminSession)

	await waitFor(() => expect(readProbe()).toHaveTextContent('failed'))
	await expectNoTimeLimit(signals)
})

test('stops a busy dialog at the time limit the admin settings serve', async () => {
	server.use(
		graphql.query('AdminSettings', () =>
			HttpResponse.json({
				data: {
					adminSettings: {
						__typename: 'AdminSettings',
						toastMilliseconds: 6000,
						toastActionMilliseconds: 10000,
						dialogWriteTimeoutMilliseconds: 45000,
						listPageSizes: [10, 20, 50, 100],
						listPageSize: 20,
						contactPageCap: 200,
						formatLocale: 'es-ES',
					},
				},
			}),
		),
	)
	const signals = renderHost(adminSession)
	await waitFor(() => expect(readProbe()).toHaveTextContent('served'))

	await pressDeleteOnAHeldClock()
	await runClock(44999)
	expect(signals[0]?.aborted).toBe(false)
	await runClock(1)
	expect(signals[0]?.aborted).toBe(true)
	await runClock(1)

	expect(screen.getByText(timedOutWords)).toBeInTheDocument()
})
