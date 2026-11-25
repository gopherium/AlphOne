// SPDX-License-Identifier: AGPL-3.0-or-later

import { adoptSession, createAuthQueryClient, sessionQueryKey } from '@gopherium/react-auth'
import type { User } from '@gopherium/react-auth'
import { QueryClientProvider } from '@tanstack/react-query'
import type { QueryClient } from '@tanstack/react-query'
import { act, render, screen, waitFor } from '@testing-library/react'
import type { ReactNode } from 'react'
import { gql } from 'urql'
import { expect, onTestFinished, test, vi } from 'vitest'

import { AccountGraph, graphKeysOf, useGraph, useGraphQuery, useSession } from '../index'
import type { GraphClient, GraphKeys } from '../index'
import { HttpResponse, adminSession, graphql, memberSession, seedSession, server } from '../testing'

/** noteQuery reads the one note the signed-in account keeps. */
const noteQuery = gql`
	query AccountNote {
		note {
			id
			text
		}
	}
`

/** keyedQuery reads one embedded value of a type without an id. */
const keyedQuery = gql`
	query AccountKeyed {
		keyed {
			value
		}
	}
`

/** Shows the note the signed-in account keeps, read cache first. */
function NoteProbe() {
	const [result] = useGraphQuery({ query: noteQuery })
	return <p>{result.data?.note.text ?? 'no note'}</p>
}

/** Shows the embedded value the graph serves. */
function KeyedProbe() {
	const [result] = useGraphQuery({ query: keyedQuery })
	return <p>{result.data?.keyed.value ?? 'no value'}</p>
}

/**
 * Shows the signed-in account's name, keeping each graph client it is served.
 * @param props - The list each served graph client joins.
 * @returns The account's name.
 */
function GraphProbe({ seen }: { seen: GraphClient[] }) {
	const session = useSession()
	seen.push(useGraph())
	return <p>{session?.name ?? 'nobody'}</p>
}

/**
 * Renders the tree below an account graph, signed in as the given account.
 * @param user - The signed-in account, or null for nobody.
 * @param tree - The screens reading the graph.
 * @param keys - The graph keys the account graph files answers by.
 * @returns The query client holding the session.
 */
function renderAccount(
	user: (User & { role?: string }) | null,
	tree: ReactNode = <NoteProbe />,
	keys: GraphKeys = graphKeysOf([]),
): QueryClient {
	const client = createAuthQueryClient({ queries: { retry: false, staleTime: Infinity } })
	seedSession(client, user)
	render(
		<QueryClientProvider client={client}>
			<AccountGraph keys={keys}>{tree}</AccountGraph>
		</QueryClientProvider>,
	)
	return client
}

/**
 * Serves the note of whichever account the returned call names, counting each read.
 * @returns The call naming the account whose note the graph serves, beside the count of reads so far.
 */
function serveNotes(): { serve: (text: string) => void; asked: () => number } {
	let asked = 0
	let text = 'first account note'
	server.use(
		graphql.query('AccountNote', () => {
			asked += 1
			return HttpResponse.json({ data: { note: { __typename: 'Note', id: 'note-1', text } } })
		}),
	)
	return {
		serve: (next) => {
			text = next
		},
		asked: () => asked,
	}
}

test('reads afresh from the network for the next account that signs in', async () => {
	const notes = serveNotes()
	const client = renderAccount(adminSession)
	expect(await screen.findByText('first account note')).toBeInTheDocument()

	notes.serve('second account note')
	await act(() => adoptSession(client, memberSession))

	expect(await screen.findByText('second account note')).toBeInTheDocument()
	expect(notes.asked()).toBe(2)
})

test('reads afresh when the session turns to another account without a sign-in', async () => {
	const notes = serveNotes()
	const client = renderAccount(adminSession)
	expect(await screen.findByText('first account note')).toBeInTheDocument()

	notes.serve('second account note')
	act(() => client.setQueryData(sessionQueryKey, memberSession))

	expect(await screen.findByText('second account note')).toBeInTheDocument()
	expect(notes.asked()).toBe(2)
})

test('keeps its graph client while the same account stays signed in', async () => {
	const notes = serveNotes()
	const seen: GraphClient[] = []
	const client = renderAccount(
		adminSession,
		<>
			<NoteProbe />
			<GraphProbe seen={seen} />
		</>,
	)
	expect(await screen.findByText('first account note')).toBeInTheDocument()

	act(() => client.setQueryData(sessionQueryKey, { ...adminSession, name: 'Maria Perez' }))

	expect(await screen.findByText('Maria Perez')).toBeInTheDocument()
	expect(screen.getByText('first account note')).toBeInTheDocument()
	expect(new Set(seen).size).toBe(1)
	expect(notes.asked()).toBe(1)
})

test('hands the next account a doorbell and an event stream of its own', async () => {
	const seen: GraphClient[] = []
	const client = renderAccount(adminSession, <GraphProbe seen={seen} />)
	const first = seen[0]

	await act(() => adoptSession(client, memberSession))

	await waitFor(() => expect(screen.getByText(memberSession.name)).toBeInTheDocument())
	const next = seen.at(-1)
	expect(next).not.toBe(first)
	expect(next?.client).not.toBe(first.client)
	expect(next?.refetch).not.toBe(first.refetch)
	expect(next?.onStreamOpen).not.toBe(first.onStreamOpen)
})

test('signs the account out when the graph rejects its session', async () => {
	server.use(
		graphql.query('AccountNote', () =>
			HttpResponse.json({
				data: null,
				errors: [{ message: 'no session', extensions: { code: 'UNAUTHENTICATED' } }],
			}),
		),
	)
	const client = renderAccount(adminSession)

	await waitFor(() => expect(client.getQueryData(sessionQueryKey)).toBeNull())
})

test('files the answers by the graph keys it is handed', async () => {
	const warn = vi.spyOn(console, 'warn').mockImplementation(() => {})
	onTestFinished(() => warn.mockRestore())
	server.use(
		graphql.query('AccountKeyed', () =>
			HttpResponse.json({ data: { keyed: { __typename: 'AccountKeyedEmbed', value: 'keyed value' } } }),
		),
	)
	renderAccount(
		adminSession,
		<KeyedProbe />,
		graphKeysOf([{ id: 'probe', graphKeys: { AccountKeyedEmbed: () => null } }]),
	)

	expect(await screen.findByText('keyed value')).toBeInTheDocument()
	expect(warn).not.toHaveBeenCalled()
})

test('serves the tree a graph client while nobody is signed in', () => {
	const seen: GraphClient[] = []
	renderAccount(null, <GraphProbe seen={seen} />)

	expect(screen.getByText('nobody')).toBeInTheDocument()
	expect(seen.length).toBeGreaterThan(0)
})
