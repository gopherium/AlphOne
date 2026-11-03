// SPDX-License-Identifier: AGPL-3.0-or-later

import { HttpResponse, graphql, server } from '@alphone/frontend-sdk/testing'
import { screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { beforeEach, expect, test } from 'vitest'

import { busyClasses, renderAt } from './render'

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

test('shows the save button busy while the choice is on its way', async () => {
	const busy = busyClasses()
	expect(busy.length).toBeGreaterThan(0)
	server.use(graphql.mutation('SetLocale', () => new Promise(() => {})))
	renderAt('/language')

	await userEvent.click(await screen.findByRole('combobox', { name: 'Language' }))
	await userEvent.click(await screen.findByRole('option', { name: 'es-ES' }))
	await userEvent.click(screen.getByRole('button', { name: 'Save' }))

	const save = screen.getByRole('button', { name: 'Save' })
	await waitFor(() => expect([...save.classList]).toEqual(expect.arrayContaining(busy)))
	expect(save).toHaveAttribute('aria-disabled', 'true')
})

test('says in a toast that a refused choice was not saved, in the words of the server', async () => {
	server.use(
		graphql.mutation('SetLocale', () =>
			HttpResponse.json({ errors: [{ message: 'that locale is not served', extensions: { code: 'VALIDATION' } }] })),
	)
	renderAt('/language')

	await userEvent.click(await screen.findByRole('combobox', { name: 'Language' }))
	await userEvent.click(await screen.findByRole('option', { name: 'es-ES' }))
	await userEvent.click(screen.getByRole('button', { name: 'Save' }))

	expect(
		await screen.findByText('Could not save the language. that locale is not served', { selector: '.godmin-toast' }),
	).toBeInTheDocument()
	expect(screen.queryByRole('alert')).not.toBeInTheDocument()
	expect(screen.getByRole('combobox', { name: 'Language' })).toHaveTextContent('es-ES')
	await waitFor(() => expect(screen.getByRole('button', { name: 'Save' })).not.toHaveAttribute('aria-disabled', 'true'))
})

test('reports an unreadable language list inside the page shell', async () => {
	server.use(
		graphql.query('SupportedLocales', () =>
			HttpResponse.json({ errors: [{ message: 'boom' }] })),
	)
	renderAt('/language')

	expect(await screen.findByText('The languages could not be read.')).toBeInTheDocument()
	expect(screen.getByRole('heading', { level: 2, name: 'Language' })).toBeInTheDocument()
})

test('says in a toast that a save that answered nothing at all was not saved', async () => {
	server.use(graphql.mutation('SetLocale', () => HttpResponse.json({ data: null })))
	renderAt('/language')

	await userEvent.click(await screen.findByRole('combobox', { name: 'Language' }))
	await userEvent.click(await screen.findByRole('option', { name: 'es-ES' }))
	await userEvent.click(screen.getByRole('button', { name: 'Save' }))

	expect(await screen.findByText('Could not save the language.', { selector: '.godmin-toast' })).toBeInTheDocument()
	expect(screen.queryByRole('alert')).not.toBeInTheDocument()
})

test('takes back the word that the language changes when a later save fails', async () => {
	let saves = 0
	server.use(
		graphql.mutation('SetLocale', ({ variables }) => {
			saves += 1
			return saves === 1
				? HttpResponse.json({ data: { setLocale: variables.locale } })
				: HttpResponse.json({ errors: [{ message: 'internal error' }] })
		}),
	)
	renderAt('/language')
	await screen.findByRole('combobox', { name: 'Language' })

	await userEvent.click(screen.getByRole('button', { name: 'Save' }))
	await screen.findByText('The language changes when the page next loads.')
	await waitFor(() => expect(screen.getByRole('button', { name: 'Save' })).not.toHaveAttribute('aria-disabled', 'true'))
	await userEvent.click(screen.getByRole('button', { name: 'Save' }))

	expect(await screen.findByText('Could not save the language.', { selector: '.godmin-toast' })).toBeInTheDocument()
	expect(screen.queryByText('The language changes when the page next loads.')).not.toBeInTheDocument()
})
