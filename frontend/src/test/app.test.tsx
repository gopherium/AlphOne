// SPDX-License-Identifier: AGPL-3.0-or-later

import { graphKeysOf } from '@alphone/frontend-sdk'
import { HttpResponse, adminSession, graphql, memberSession, server } from '@alphone/frontend-sdk/testing'
import { configureAuthTransport, createAuthQueryClient, sessionQueryKey } from '@gopherium/react-auth'
import type { User } from '@gopherium/react-auth'
import { createMemoryHistory } from '@tanstack/react-router'
import { render, screen, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { StrictMode } from 'react'
import type { ReactNode } from 'react'
import { beforeAll, beforeEach, expect, onTestFinished, test } from 'vitest'

import { App } from '../App'
import { graphAuthTransport } from '../auth/graphTransport'
import { plugins } from '../plugins'
import { createAppRouter } from '../router'
import { gate } from './gate'
import { dayTasks, taskServer } from './tasksScreen'
import type { TaskRow } from './tasksScreen'

configureAuthTransport(graphAuthTransport)

/** Account is one account the graph signs in. */
type Account = User & { role?: string }

/** QUIET is how long in milliseconds a test waits for a request the app must never send. */
const QUIET = 60

/** NO_SESSION is the graph's answer to a caller who is not signed in. */
const NO_SESSION = { data: null, errors: [{ message: 'no session', extensions: { code: 'UNAUTHENTICATED' } }] }

/** ACCOUNTS are the accounts the graph signs in, found by the email typed. */
const ACCOUNTS: Account[] = [adminSession, memberSession]

/** TOKEN_NAMES are the names of the one API token each account holds, by its email. */
const TOKEN_NAMES: Record<string, string> = {
	[adminSession.email]: 'Admin token',
	[memberSession.email]: 'Member token',
}

/** signedIn is the account the graph serves the session of, null for nobody. */
let signedIn: Account | null = null

/** tasks are the tasks of the day the task server holds. */
let tasks: TaskRow[] = []

const { serveDay } = taskServer(
	() => tasks,
	() => [],
)

beforeAll(async () => {
	await import('../users/TokensScreen')
})

beforeEach(() => {
	signedIn = null
	tasks = dayTasks()
})

/**
 * Serves the session, the sign-in, the sign-out and the version, signed in as the given account when the app opens.
 * @param account - The account signed in when the app opens, or null for nobody.
 */
function serveAccounts(account: Account | null) {
	signedIn = account
	server.use(
		graphql.query('Me', () => HttpResponse.json(signedIn === null ? NO_SESSION : { data: { me: signedIn } })),
		graphql.mutation('Login', ({ variables }) => {
			signedIn = ACCOUNTS.find((each) => each.email === variables.email) ?? null
			return HttpResponse.json({ data: { login: { me: signedIn } } })
		}),
		graphql.mutation('Logout', () => {
			signedIn = null
			return HttpResponse.json({ data: { logout: true } })
		}),
		graphql.query('Version', () => HttpResponse.json({ data: { version: '0.1.0' } })),
	)
}

/**
 * Serves each account the one API token it holds, the member's answer waiting on the given gate.
 * @param memberRead - The gate the member's answer waits on.
 */
function serveTokens(memberRead: Promise<void>) {
	server.use(
		graphql.query('ApiTokens', async () => {
			const email = signedIn?.email ?? ''
			if (email === memberSession.email) {
				await memberRead
			}
			return HttpResponse.json({
				data: {
					apiTokens: [
						{
							__typename: 'ApiToken',
							id: `0198b2f0-0000-7000-8000-00000000${email === memberSession.email ? '0502' : '0501'}`,
							name: TOKEN_NAMES[email],
							scopes: ['tokens:read'],
							createdAt: '2026-07-06T10:00:00Z',
							lastUsedAt: null,
							expiresAt: null,
						},
					],
				},
			})
		}),
	)
}

/**
 * Records the name of every graph operation the app sends.
 * @returns The names sent so far, in order.
 */
function recordOperations(): string[] {
	const asked: string[] = []
	server.use(
		graphql.operation(({ operationName }) => {
			asked.push(operationName)
		}),
	)
	return asked
}

/**
 * Renders the admin as the entry renders it, under the strict checks, its router at one route, the session never stale.
 * @param path - The route the memory history starts on.
 * @param publicScreen - The public link screen shown in place of the admin, or null for the admin.
 */
function renderApp(path: string, publicScreen: ReactNode = null) {
	const queryClient = createAuthQueryClient({ queries: { retry: false } })
	queryClient.setQueryDefaults(sessionQueryKey, { staleTime: Infinity })
	render(
		<StrictMode>
			<App
				queryClient={queryClient}
				router={createAppRouter(createMemoryHistory({ initialEntries: [path] }))}
				links={[]}
				publicScreen={publicScreen}
				graphKeys={graphKeysOf(plugins)}
			/>
		</StrictMode>,
	)
}

/**
 * Signs in from the login screen as the account holding the given email.
 * @param email - The email typed.
 */
async function logIn(email: string) {
	await userEvent.type(await screen.findByLabelText('Email'), email)
	await userEvent.type(screen.getByLabelText('Password'), 'correct horse battery')
	await userEvent.click(screen.getByRole('button', { name: 'Log in' }))
}

/** Signs out from the account panel and waits for the login screen. */
async function logOut() {
	await userEvent.click(await screen.findByRole('button', { name: 'Log out' }))
	await screen.findByLabelText('Email')
}

/** Waits long enough for a request the app must never send. */
async function waitQuiet() {
	await new Promise((resolve) => setTimeout(resolve, QUIET))
}

test('holds the admin on the boot spinner while the session loads', async () => {
	const session = gate()
	onTestFinished(session.release)
	server.use(
		graphql.query('Me', async () => {
			await session.held
			return HttpResponse.json(NO_SESSION)
		}),
	)
	renderApp('/tasks')

	const status = await screen.findByRole('status')
	expect(status).toHaveTextContent('Loading…')
	expect(status.closest('.godmin-loading-screen')).not.toBeNull()
})

test('shows the login screen while nobody is signed in', async () => {
	serveAccounts(null)
	renderApp('/tasks')

	expect(await screen.findByLabelText('Email')).toBeInTheDocument()
	expect(screen.getByRole('button', { name: 'Log in' })).toBeInTheDocument()
})

test('asks the graph for nothing but the session before anyone signs in', async () => {
	serveAccounts(null)
	const asked = recordOperations()
	renderApp('/tasks')

	await screen.findByLabelText('Email')
	await waitQuiet()

	expect([...new Set(asked)]).toEqual(['Me'])
})

test('shows a public link screen in place of the admin', async () => {
	serveAccounts(null)
	renderApp('/activate', <p>Set a password</p>)

	expect(await screen.findByText('Set a password')).toBeInTheDocument()
	await waitQuiet()
	expect(screen.queryByLabelText('Email')).toBeNull()
})

test('reads the tokens afresh for the next account that signs in on the same tab', async () => {
	serveAccounts(adminSession)
	const memberRead = gate()
	onTestFinished(memberRead.release)
	serveTokens(memberRead.held)
	renderApp('/users/tokens')
	expect(await screen.findByText('Admin token')).toBeInTheDocument()

	await logOut()
	await logIn(memberSession.email)

	expect(await screen.findByRole('heading', { level: 2, name: 'API tokens' })).toBeInTheDocument()
	expect(screen.queryByText('Admin token')).toBeNull()
	memberRead.release()
	expect(await screen.findByText('Member token')).toBeInTheDocument()
	expect(screen.queryByText('Admin token')).toBeNull()
})

test('closes the toasts when the account signs out', async () => {
	serveAccounts(adminSession)
	serveDay()
	renderApp('/tasks')
	const row = await screen.findByRole('listitem', { name: 'Call the supplier' })
	await userEvent.click(within(row).getByRole('checkbox', { name: 'Complete' }))
	expect(await screen.findByRole('button', { name: 'Undo' })).toBeInTheDocument()

	await logOut()

	expect(screen.queryByText('Task completed.')).toBeNull()
	expect(screen.queryByRole('button', { name: 'Undo' })).toBeNull()
})

test('asks again which plugins hide their screens for the next account that signs in', async () => {
	serveAccounts(adminSession)
	const asked: string[] = []
	server.use(
		graphql.query('HiddenPlugins', () => {
			asked.push(signedIn?.email ?? '')
			return HttpResponse.json({ data: { hiddenPlugins: [] } })
		}),
	)
	renderApp('/tasks')
	await screen.findByRole('heading', { level: 2, name: 'Tasks' })

	await logOut()
	await logIn(memberSession.email)
	await screen.findByRole('heading', { level: 2, name: 'Tasks' })
	await waitQuiet()

	expect(asked).toEqual([adminSession.email, memberSession.email])
})
