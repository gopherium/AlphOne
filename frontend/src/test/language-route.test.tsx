// SPDX-License-Identifier: AGPL-3.0-or-later

import { HttpResponse, graphql, server } from '@alphone/frontend-sdk/testing'
import { screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { beforeEach, expect, test } from 'vitest'

import { renderAt } from './render'

beforeEach(() => {
	server.use(
		graphql.query('SupportedLocales', () =>
			HttpResponse.json({ data: { supportedLocales: ['en-US', 'es-ES'] } })),
	)
})

test('offers every locale the server serves', async () => {
	renderAt('/language')

	await userEvent.click(await screen.findByRole('combobox', { name: 'Language' }))

	expect(await screen.findByRole('option', { name: 'en-US' })).toBeInTheDocument()
	expect(screen.getByRole('option', { name: 'es-ES' })).toBeInTheDocument()
})

test('stores the chosen locale and confirms the change', async () => {
	let stored = ''
	server.use(
		graphql.mutation('SetLocale', ({ variables }) => {
			stored = variables.locale as string
			return HttpResponse.json({ data: { setLocale: variables.locale } })
		}),
	)
	renderAt('/language')

	await userEvent.click(await screen.findByRole('combobox', { name: 'Language' }))
	await userEvent.click(await screen.findByRole('option', { name: 'es-ES' }))
	await userEvent.click(screen.getByRole('button', { name: 'Save' }))

	expect(await screen.findByRole('status')).toHaveTextContent('The language changes when the page next loads.')
	expect(stored).toBe('es-ES')
})

test('reports a refused choice rather than failing quietly', async () => {
	server.use(
		graphql.mutation('SetLocale', () =>
			HttpResponse.json({ errors: [{ message: 'that locale is not served' }] })),
	)
	renderAt('/language')

	await userEvent.click(await screen.findByRole('combobox', { name: 'Language' }))
	await userEvent.click(await screen.findByRole('option', { name: 'es-ES' }))
	await userEvent.click(screen.getByRole('button', { name: 'Save' }))

	expect(await screen.findByRole('alert')).toHaveTextContent('that locale is not served')
})

test('reports an unreadable language list inside the page shell', async () => {
	server.use(
		graphql.query('SupportedLocales', () =>
			HttpResponse.json({ errors: [{ message: 'boom' }] })),
	)
	renderAt('/language')

	expect(await screen.findByText('The languages could not be read.')).toBeInTheDocument()
	expect(screen.getByRole('heading', { level: 1, name: 'Language' })).toBeInTheDocument()
})

test('reports a save that answered nothing at all', async () => {
	server.use(graphql.mutation('SetLocale', () => HttpResponse.json({ data: null })))
	renderAt('/language')

	await userEvent.click(await screen.findByRole('combobox', { name: 'Language' }))
	await userEvent.click(await screen.findByRole('option', { name: 'es-ES' }))
	await userEvent.click(screen.getByRole('button', { name: 'Save' }))

	expect(await screen.findByRole('alert')).toHaveTextContent('The choice could not be saved.')
})
