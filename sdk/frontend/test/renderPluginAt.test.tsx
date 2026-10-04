// SPDX-License-Identifier: AGPL-3.0-or-later

import { createRoute } from '@tanstack/react-router'
import { screen } from '@testing-library/react'
import { expect, onTestFinished, test } from 'vitest'

import { useSession } from '../index'
import type { FrontendPlugin } from '../index'
import { HttpResponse, adminSession, http, renderPluginAt, server } from '../testing'

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

test('hands back the router the plugin screen is mounted in', async () => {
	const { router } = renderPluginAt(sessionPlugin, '/signed-in', { session: adminSession })

	await screen.findByText(`Signed in as ${adminSession.name}`)

	expect(router.state.location.pathname).toBe('/signed-in')
})
