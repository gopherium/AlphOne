// SPDX-License-Identifier: AGPL-3.0-or-later

import {
	AccountGraph,
	FormatLocaleGate,
	GraphProvider,
	createGraphClient,
	graphKeysOf,
	useAdminSettings,
} from '@alphone/frontend-sdk'
import type { AdminSettingsRead } from '@alphone/frontend-sdk'
import { HttpResponse, adminSession, graphql, seedSession, server } from '@alphone/frontend-sdk/testing'
import { ConfirmDialog } from '@gopherium/godmin'
import { AuthGate, createAuthQueryClient } from '@gopherium/react-auth'
import type { User } from '@gopherium/react-auth'
import { QueryClientProvider } from '@tanstack/react-query'
import type { QueryClient } from '@tanstack/react-query'
import { act, fireEvent, render, screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import type { ReactNode } from 'react'
import { afterEach, expect, test, vi } from 'vitest'

import { AdminHost } from '../AdminHost'

/** The words godmin shows once a write ran past its time limit. */
const timedOutWords = 'Could not get a valid response from the server.'

/** The longest time limit in milliseconds godmin takes short of none. */
const longestLimit = 2 ** 31 - 1

/** A time limit in milliseconds far shorter than any test. */
const shortLimit = 20

/** How long in milliseconds a test waits for a request the host must never send. */
const quiet = 60

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
 * Builds the query client a render reads through, signed in as the given account.
 * @param user - The signed-in account, or null for nobody.
 * @returns The query client.
 */
function sessionClient(user: (User & { role?: string }) | null): QueryClient {
	const client = createAuthQueryClient({ queries: { retry: false, staleTime: Infinity } })
	seedSession(client, user)
	return client
}

/**
 * Renders the admin host around the given tree, with no graph client above it.
 * @param children - The tree to host.
 * @param client - The query client the tree reads through.
 */
function renderHosted(children: ReactNode, client: QueryClient) {
	render(
		<QueryClientProvider client={client}>
			<AdminHost>{children}</AdminHost>
		</QueryClientProvider>,
	)
}

/**
 * Renders the admin host around a waiting delete, signed in as the given account.
 * @param user - The signed-in account.
 * @returns The signals the writes of the delete were handed, in order.
 */
function renderHost(user: User & { role?: string }): AbortSignal[] {
	const signals: AbortSignal[] = []
	render(
		<QueryClientProvider client={sessionClient(user)}>
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

/**
 * Serves the admin settings with the given write time limit.
 * @param milliseconds - How long in milliseconds a busy dialog waits on its write.
 * @returns The names of the operations the graph was asked, in order.
 */
function serveWriteTimeout(milliseconds: number): string[] {
	const asked: string[] = []
	server.use(
		graphql.query('AdminSettings', () => {
			asked.push('AdminSettings')
			return HttpResponse.json({
				data: {
					adminSettings: {
						__typename: 'AdminSettings',
						toastMilliseconds: 6000,
						toastActionMilliseconds: 10000,
						dialogWriteTimeoutMilliseconds: milliseconds,
						listPageSizes: [10, 20, 50, 100],
						listPageSize: 20,
						contactPageCap: 200,
						formatLocale: 'es-ES',
					},
				},
			})
		}),
	)
	return asked
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
	serveWriteTimeout(45000)
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

test('leaves the settings for the screens to load', async () => {
	const asked = serveWriteTimeout(shortLimit)
	const signals: AbortSignal[] = []
	renderHosted(<WaitingDelete onSignal={(signal) => signals.push(signal)} />, sessionClient(adminSession))

	await expectNoTimeLimit(signals)
	vi.useRealTimers()
	await new Promise((resolve) => setTimeout(resolve, quiet))

	expect(asked).toEqual([])
})

test('gives up on a dialog write once the time the admin settings serve runs out after a sign-in', async () => {
	serveWriteTimeout(shortLimit)
	renderHosted(
		<AuthGate
			loginScreen={(onLogin) => (
				<button type="button" onClick={() => void onLogin(adminSession)}>
					Log in
				</button>
			)}
		>
			<AccountGraph keys={graphKeysOf([])}>
				<FormatLocaleGate loading={null}>
					<WaitingDelete onSignal={() => {}} />
				</FormatLocaleGate>
			</AccountGraph>
		</AuthGate>,
		sessionClient(null),
	)

	await userEvent.click(screen.getByRole('button', { name: 'Log in' }))
	await userEvent.click(await screen.findByRole('button', { name: 'Delete' }))

	expect(await screen.findByText(timedOutWords)).toBeVisible()
})
