// SPDX-License-Identifier: AGPL-3.0-or-later

import { GraphProvider, createGraphClient } from '@alphone/frontend-sdk'
import type { ContactPanel, FrontendPlugin } from '@alphone/frontend-sdk'
import { HttpResponse, adminSession, graphql, seedSession, server } from '@alphone/frontend-sdk/testing'
import { createAuthQueryClient } from '@gopherium/react-auth'
import { QueryClientProvider } from '@tanstack/react-query'
import { render, screen, waitFor } from '@testing-library/react'
import { lazy } from 'react'
import type { ReactElement } from 'react'
import { expect, test } from 'vitest'

import { ContactPanels } from '../ContactPanels'
import { panelsOf } from '../panels'
import type { PluginPanel } from '../panels'

const contactID = '0198c000-0000-7000-8000-000000000401'

/**
 * Returns a panel that names itself and the contact it renders for.
 * @param id - The panel's own id.
 * @param plugin - The id of the plugin contributing it, the panel's own id when absent.
 * @returns The panel beside its plugin's id.
 */
function panel(id: string, plugin = id): PluginPanel {
	return { id, plugin, Panel: ({ contactId }) => <p>{`${id} for ${contactId}`}</p> }
}

/**
 * Renders a tree below a graph client, signed in as an admin.
 * @param tree - The tree to render.
 * @returns The rendered result.
 */
function renderSignedIn(tree: ReactElement) {
	const client = createAuthQueryClient({ queries: { retry: false, staleTime: Infinity } })
	seedSession(client, adminSession)
	return render(
		<QueryClientProvider client={client}>
			<GraphProvider graph={createGraphClient({ onSessionExpired: () => {} })}>{tree}</GraphProvider>
		</QueryClientProvider>,
	)
}

/**
 * Serves the given ids as the plugins that hide their screens in the workspace.
 * @param ids - The ids the graph answers.
 */
function serveHidden(ids: string[]) {
	server.use(graphql.query('HiddenPlugins', () => HttpResponse.json({ data: { hiddenPlugins: ids } })))
}

test('every contributed panel renders for the contact', () => {
	renderSignedIn(
		<ContactPanels contactId={contactID} panels={[panel('fields'), panel('notes')]} />,
	)

	expect(screen.getByText(`fields for ${contactID}`)).toBeInTheDocument()
	expect(screen.getByText(`notes for ${contactID}`)).toBeInTheDocument()
})

test('keeps every other panel on screen while one panel loads', () => {
	const loading: PluginPanel = { id: 'fields', plugin: 'fields', Panel: lazy(() => new Promise<never>(() => {})) }

	renderSignedIn(<ContactPanels contactId={contactID} panels={[loading, panel('notes')]} />)

	expect(screen.getByText(`notes for ${contactID}`)).toBeInTheDocument()
})

test('no panels render nothing at all', () => {
	const { container } = renderSignedIn(<ContactPanels contactId={contactID} panels={[]} />)

	expect(container).toBeEmptyDOMElement()
})

test('skips the panels of a plugin once the graph names it as hiding its screens, keeping the rest', async () => {
	serveHidden(['notes'])
	renderSignedIn(<ContactPanels contactId={contactID} panels={[panel('fields'), panel('notes')]} />)

	expect(screen.getByText(`notes for ${contactID}`)).toBeInTheDocument()
	await waitFor(() => expect(screen.queryByText(`notes for ${contactID}`)).not.toBeInTheDocument())
	expect(screen.getByText(`fields for ${contactID}`)).toBeInTheDocument()
})

test('skips a panel by the plugin contributing it, whatever the panel calls itself', async () => {
	serveHidden(['notes'])
	renderSignedIn(
		<ContactPanels contactId={contactID} panels={[panel('summary', 'notes'), panel('notes', 'fields')]} />,
	)

	await waitFor(() => expect(screen.queryByText(`summary for ${contactID}`)).not.toBeInTheDocument())
	expect(screen.getByText(`notes for ${contactID}`)).toBeInTheDocument()
})

test('keeps each contributed panel beside the id of its plugin, whatever the panel calls itself', () => {
	const summary: ContactPanel = { id: 'summary', Panel: () => null }
	const fields: ContactPanel = { id: 'fields', Panel: () => null }
	const contributing: FrontendPlugin[] = [
		{ id: 'notes', routes: () => [], nav: [], contactPanels: [summary] },
		{ id: 'quiet', routes: () => [], nav: [] },
		{ id: 'fields', routes: () => [], nav: [], contactPanels: [fields] },
	]

	expect(panelsOf(contributing)).toEqual([
		{ id: 'summary', plugin: 'notes', Panel: summary.Panel },
		{ id: 'fields', plugin: 'fields', Panel: fields.Panel },
	])
})
