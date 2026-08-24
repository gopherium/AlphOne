// SPDX-License-Identifier: AGPL-3.0-or-later

import { HttpResponse, graphql, server } from '@alphone/frontend-sdk/testing'
import { displayLocale } from '@gopherium/gottext'
import { resetLocale } from '@gopherium/gottext/testing'
import { afterEach, expect, test } from 'vitest'

import { fetchLocale } from '../i18n/api'
import { DOMAIN, declaredEntries, localeEntries, startAppLocale } from '../i18n/start'
import { appErrorTemplates, declaredTemplates } from '../i18n/errors'

afterEach(() => {
	resetLocale()
})

test('answers the locale the graph resolves', async () => {
	server.use(graphql.query('AppLocale', () => HttpResponse.json({ data: { locale: 'es-ES' } })))

	expect(await fetchLocale()).toBe('es-ES')
})

test('answers the default when the graph refuses the ask', async () => {
	server.use(
		graphql.query('AppLocale', () =>
			HttpResponse.json({ errors: [{ message: 'authentication required' }] })),
	)

	expect(await fetchLocale()).toBe('en-US')
})

test('answers the default when the server cannot be reached', async () => {
	server.use(graphql.query('AppLocale', () => HttpResponse.error()))

	expect(await fetchLocale()).toBe('en-US')
})

test('answers the default when the graph answers nothing readable', async () => {
	server.use(graphql.query('AppLocale', () => HttpResponse.json({ data: {} })))

	expect(await fetchLocale()).toBe('en-US')
})

test('settles the interface on the locale the graph resolves', async () => {
	server.use(graphql.query('AppLocale', () => HttpResponse.json({ data: { locale: 'es-ES' } })))

	const settled = await startAppLocale()

	expect(settled).toBe('es-ES')
	expect(displayLocale()).toBe('es-ES')
})

test('settles on the default when the graph cannot say', async () => {
	server.use(graphql.query('AppLocale', () => HttpResponse.error()))

	expect(await startAppLocale()).toBe('en-US')
	expect(displayLocale()).toBe('en-US')
})

test('names the domain AlphOne strings answer under', () => {
	expect(DOMAIN).toBe('alphone')
})

test('loads one entry per domain beside the react-auth pair', () => {
	const domains = localeEntries().map((entry) => entry.domain)

	expect(domains).toEqual([
		'alphone',
		'alphone-fields',
		'alphone-importer',
		'alphone-whatsapp',
		'gopherium-react-auth',
	])
})

test('skips a plugin declaring no domain of its own', () => {
	const declared = declaredEntries([{ id: 'bare', routes: () => [], nav: [] }])

	expect(declared).toEqual([])
})

test('renders core and plugin reasons from one merged map', () => {
	const merged = appErrorTemplates()

	expect(merged.contact_name_required).toBe('A contact needs a name.')
	expect(merged.field_name_taken).toBe('Another field already holds that name.')
	expect(merged.import_not_found).toBe('That import no longer exists.')
	expect(merged.message_content_required).toBe('Write something to send.')
})

test('skips a plugin declaring no templates of its own', () => {
	expect(declaredTemplates([{ id: 'bare', routes: () => [], nav: [] }])).toEqual({})
})

test('points the graph seam at the merged templates', async () => {
	const { configureAppErrorText } = await import('../i18n/errors')
	const { errorFallback } = await import('../i18n/errorTemplates')

	configureAppErrorText()

	expect(errorFallback()).toBe('Something went wrong. Try again.')
})
