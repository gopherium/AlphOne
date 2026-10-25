// SPDX-License-Identifier: AGPL-3.0-or-later

import { HttpResponse, graphql, server, setViewport } from '@alphone/frontend-sdk/testing'
import { resetLocale } from '@gopherium/gottext/testing'
import { screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { beforeAll, beforeEach, expect, onTestFinished, test } from 'vitest'

import { handlers } from '@alphone/plugin-whatsapp/handlers'
import { startAppLocale } from '../i18n/start'
import { renderAt } from './render'

/** The name godmin gives the button that opens the sidebar on a phone. */
const MENU_BUTTON = 'Open navigation panel'

beforeAll(async () => {
	await import('../contacts/ContactsScreen')
})

beforeEach(() => server.use(...handlers))

test('holds the navigation behind a menu button on a narrow viewport', async () => {
	setViewport({ matches: true })
	renderAt('/tasks')

	await screen.findByRole('heading', { name: 'Tasks' })

	expect(screen.getByRole('button', { name: MENU_BUTTON })).toBeInTheDocument()
	expect(screen.queryByRole('region', { name: 'Navigation' })).toBeNull()
})

test('opens the navigation in a drawer named for it, holding no region of its own', async () => {
	setViewport({ matches: true })
	renderAt('/tasks')

	await userEvent.click(await screen.findByRole('button', { name: MENU_BUTTON }))

	const drawer = await screen.findByRole('dialog', { name: 'Navigation' })
	expect(within(drawer).getByRole('link', { name: 'Contacts' })).toBeInTheDocument()
	expect(within(drawer).getByRole('button', { name: 'Log out' })).toBeInTheDocument()
	expect(within(drawer).queryByRole('region')).toBeNull()
})

test('puts the navigation away once it takes the reader somewhere', async () => {
	setViewport({ matches: true })
	renderAt('/tasks')

	await userEvent.click(await screen.findByRole('button', { name: MENU_BUTTON }))
	const drawer = await screen.findByRole('dialog')
	await userEvent.click(within(drawer).getByRole('link', { name: 'Contacts' }))

	await waitFor(() => expect(screen.queryByRole('dialog')).toBeNull())
	expect(await screen.findByRole('heading', { name: 'Contacts' })).toBeInTheDocument()
})

test('names the menu button in Spanish from the words godmin ships', async () => {
	server.use(graphql.query('AppLocale', () => HttpResponse.json({ data: { locale: 'es-ES' } })))
	await startAppLocale()
	onTestFinished(() => resetLocale())
	setViewport({ matches: true })
	renderAt('/tasks')

	expect(await screen.findByRole('button', { name: 'Abrir el panel de navegación' })).toBeInTheDocument()
})

test('keeps the sidebar beside the canvas when there is room', async () => {
	setViewport({ matches: false })
	renderAt('/tasks')

	await screen.findByRole('heading', { name: 'Tasks' })

	expect(screen.getByRole('region', { name: 'Navigation' })).toBeInTheDocument()
	expect(screen.queryByRole('button', { name: MENU_BUTTON })).toBeNull()
})
