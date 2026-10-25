// SPDX-License-Identifier: AGPL-3.0-or-later

import { useQuery } from '@tanstack/react-query'
import { createRoute } from '@tanstack/react-router'
import { act, fireEvent, screen, waitFor, within } from '@testing-library/react'
import { lazy } from 'react'
import { expect, onTestFinished, test } from 'vitest'

import { MANAGE_USERS, NavigationScreen, useSession } from '../index'
import type { FrontendPlugin, MenuItem } from '../index'
import { HttpResponse, adminSession, http, memberSession, renderPluginAt, server } from '../testing'

/** The path the host serves the session at. */
const SESSION_PATH = '/api/auth/session'

/** Renders the name of the signed-in account, or that nobody signed in. */
function SignedInScreen() {
	const session = useSession()
	return <p>{session === null ? 'Nobody signed in' : `Signed in as ${session.name}`}</p>
}

/** A plugin mounting the signed-in screen at /signed-in. */
const sessionPlugin: FrontendPlugin = {
	id: 'signed-in',
	nav: [],
	routes: (parent) => [
		createRoute({ getParentRoute: () => parent, path: '/signed-in', component: SignedInScreen }),
	],
}

/** Records the path of every request sent until the test finishes. */
function sentPaths(): string[] {
	const paths: string[] = []
	const record = ({ request }: { request: Request }) => {
		paths.push(new URL(request.url).pathname)
	}
	server.events.on('request:start', record)
	onTestFinished(() => {
		server.events.removeListener('request:start', record)
	})
	return paths
}

test('hosts a plugin screen for the session it is given, sending no session request', async () => {
	const sent = sentPaths()

	renderPluginAt(sessionPlugin, '/signed-in', { session: adminSession })

	expect(await screen.findByText(`Signed in as ${adminSession.name}`)).toBeInTheDocument()
	expect(sent).not.toContain(SESSION_PATH)
})

test('asks the server for the session when the host is given none', async () => {
	server.use(http.get(SESSION_PATH, () => HttpResponse.json(adminSession)))
	const sent = sentPaths()

	renderPluginAt(sessionPlugin, '/signed-in')

	expect(await screen.findByText(`Signed in as ${adminSession.name}`)).toBeInTheDocument()
	expect(sent).toContain(SESSION_PATH)
})

/** How many times the probe query read its data. */
const probe = { reads: 0 }

/** Renders how many times the probe query read its data. */
function ProbeScreen() {
	const { data } = useQuery({ queryKey: ['probe'], queryFn: async () => ++probe.reads })
	return <p>{`Probe read ${data ?? 0}`}</p>
}

/** Renders a page away from the probe. */
function AwayScreen() {
	return <p>Away</p>
}

/** A plugin mounting the probe at /probe and a page away from it at /away. */
const probePlugin: FrontendPlugin = {
	id: 'probe',
	nav: [],
	routes: (parent) => [
		createRoute({ getParentRoute: () => parent, path: '/probe', component: ProbeScreen }),
		createRoute({ getParentRoute: () => parent, path: '/away', component: AwayScreen }),
	],
}

test('lets a plugin query go stale as usual when the host holds a session', async () => {
	probe.reads = 0
	const { router } = renderPluginAt(probePlugin, '/probe', { session: adminSession })
	expect(await screen.findByText('Probe read 1')).toBeInTheDocument()

	await act(() => router.navigate({ to: '/away' }))
	await screen.findByText('Away')
	await act(() => router.navigate({ to: '/probe' }))

	expect(await screen.findByText('Probe read 2')).toBeInTheDocument()
})

/** Renders the canvas of a section whose sidebar is still loading. */
function SectionScreen() {
	return <p>Section canvas</p>
}

/** A plugin mounting a section at /section, its sidebar never done loading. */
const loadingSidebarPlugin: FrontendPlugin = {
	id: 'section',
	nav: [],
	routes: (parent) => [
		createRoute({
			getParentRoute: () => parent,
			path: '/section',
			staticData: { Sidebar: lazy(() => new Promise<never>(() => {})) },
			component: SectionScreen,
		}),
	],
}

/**
 * Returns the sidebar the test host draws.
 * @returns The region named Navigation.
 */
function sidebar(): Promise<HTMLElement> {
	return screen.findByRole('region', { name: 'Navigation' })
}

test('keeps a plugin screen on the canvas while its section sidebar loads', async () => {
	renderPluginAt(loadingSidebarPlugin, '/section', { session: adminSession })

	expect(await screen.findByText('Section canvas')).toBeInTheDocument()
	const shown = await sidebar()
	expect(within(shown).queryByRole('heading')).not.toBeInTheDocument()
	expect(within(shown).queryByRole('list')).not.toBeInTheDocument()
})

/** The rows of the section plugin, one open to every reader and one asking for a capability. */
const sectionRows: MenuItem[] = [
	{ label: 'Section', to: '/section' },
	{ label: 'Section settings', to: '/section-settings', capability: MANAGE_USERS },
]

/** A plugin with two rows and a section whose sidebar is a sub menu. */
const sectionPlugin: FrontendPlugin = {
	id: 'section',
	nav: sectionRows,
	routes: (parent) => [
		createRoute({
			getParentRoute: () => parent,
			path: '/section',
			staticData: {
				Sidebar: function SectionSidebar() {
					return (
						<NavigationScreen title="Section">
							<p>section sidebar</p>
						</NavigationScreen>
					)
				},
			},
			component: function SectionCanvas() {
				return <p>section canvas</p>
			},
		}),
		createRoute({
			getParentRoute: () => parent,
			path: '/section-settings',
			component: function SectionSettings() {
				return <p>section settings</p>
			},
		}),
	],
}

test('draws the plugin rows as a list in the real sidebar, under the host title', async () => {
	renderPluginAt(sectionPlugin, '/', { session: memberSession })

	expect(await screen.findByText('Test host home')).toBeInTheDocument()
	const shown = await sidebar()
	expect(within(shown).getByRole('heading', { level: 1, name: 'Test host' })).toBeInTheDocument()
	const rows = within(within(shown).getByRole('list')).getAllByRole('listitem')
	expect(rows.map((row) => row.textContent)).toEqual(['Section'])
	expect(document.querySelector('.godmin-layout__canvas')).toContainElement(screen.getByText('Test host home'))
})

test('shows a plugin row naming a capability to a session holding it', async () => {
	renderPluginAt(sectionPlugin, '/', { session: adminSession })

	expect(await within(await sidebar()).findByRole('link', { name: 'Section settings' })).toBeInTheDocument()
})

test('asks the server for the account when the test seeds none, showing the rows that account may open', async () => {
	server.use(http.get(SESSION_PATH, () => HttpResponse.json(adminSession)))

	renderPluginAt(sectionPlugin, '/')

	expect(await within(await sidebar()).findByRole('link', { name: 'Section settings' })).toBeInTheDocument()
})

test('renders the section sub menu in the sidebar and its screen on the canvas', async () => {
	renderPluginAt(sectionPlugin, '/section', { session: memberSession })

	const shown = await sidebar()
	expect(await within(shown).findByText('section sidebar')).toBeInTheDocument()
	expect(within(shown).getByRole('heading', { level: 1, name: 'Section' })).toBeInTheDocument()
	expect(screen.getByText('section canvas')).toBeInTheDocument()
})

test('takes Back on a plugin sub menu to the host home, the focus on the row it came from', async () => {
	renderPluginAt(sectionPlugin, '/section', { session: memberSession })

	fireEvent.click(await within(await sidebar()).findByRole('link', { name: 'Back' }))

	expect(await screen.findByText('Test host home')).toBeInTheDocument()
	const row = within(await sidebar()).getByRole('link', { name: 'Section' })
	await waitFor(() => expect(row).toHaveFocus())
})

test('hands back the router the plugin screen is mounted in', async () => {
	const { router } = renderPluginAt(sessionPlugin, '/signed-in', { session: adminSession })

	await screen.findByText(`Signed in as ${adminSession.name}`)

	expect(router.state.location.pathname).toBe('/signed-in')
})
