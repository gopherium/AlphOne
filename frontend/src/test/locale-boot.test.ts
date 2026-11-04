// SPDX-License-Identifier: AGPL-3.0-or-later

import { HttpResponse, graphql, server } from '@alphone/frontend-sdk/testing'
import { LIST_CHROME_DOMAIN } from '@gopherium/godmin'
import { displayLocale } from '@gopherium/gottext'
import { resetLocale } from '@gopherium/gottext/testing'
import { __ } from '@wordpress/i18n'
import { afterEach, expect, test } from 'vitest'

import { fetchBoot } from '../i18n/api'
import { DOMAIN, declaredEntries, localeEntries, startAppLocale } from '../i18n/start'
import { plugins } from '../plugins'
import { appErrorTemplates, declaredTemplates } from '../i18n/errors'

/** PRIVACY_URL is a privacy policy address an install can publish. */
const PRIVACY_URL = 'https://example.com/legal/privacy'

/** TERMS_URL is a terms of service address an install can publish. */
const TERMS_URL = 'https://example.com/legal/terms'

/** NO_ADDRESSES is the answer of an install publishing no legal page. */
const NO_ADDRESSES = { privacyUrl: '', termsUrl: '' }

afterEach(() => {
	resetLocale()
})

test('answers the locale the graph resolves', async () => {
	server.use(graphql.query('AppLocale', () => HttpResponse.json({ data: { locale: 'es-ES' } })))

	expect((await fetchBoot()).locale).toBe('es-ES')
})

test('answers the default when the graph refuses the ask', async () => {
	server.use(
		graphql.query('AppLocale', () =>
			HttpResponse.json({ errors: [{ message: 'authentication required' }] })),
	)

	expect(await fetchBoot()).toEqual({ locale: 'en-US', legal: NO_ADDRESSES })
})

test('answers the default when the server cannot be reached', async () => {
	server.use(graphql.query('AppLocale', () => HttpResponse.error()))

	expect(await fetchBoot()).toEqual({ locale: 'en-US', legal: NO_ADDRESSES })
})

test('answers the default when the graph answers nothing readable', async () => {
	server.use(graphql.query('AppLocale', () => HttpResponse.json({ data: {} })))

	expect(await fetchBoot()).toEqual({ locale: 'en-US', legal: NO_ADDRESSES })
})

test('asks for the legal addresses in the same read as the locale', async () => {
	let asked = ''
	server.use(
		graphql.query('AppLocale', ({ query }) => {
			asked = query
			return HttpResponse.json({
				data: { locale: 'es-ES', legalLinks: { privacyUrl: PRIVACY_URL, termsUrl: TERMS_URL } },
			})
		}),
	)

	expect(await fetchBoot()).toEqual({
		locale: 'es-ES',
		legal: { privacyUrl: PRIVACY_URL, termsUrl: TERMS_URL },
	})
	expect(asked).toBe('query AppLocale { locale legalLinks { privacyUrl termsUrl } }')
})

test('answers an address the install left unset as empty', async () => {
	server.use(
		graphql.query('AppLocale', () =>
			HttpResponse.json({
				data: { locale: 'en-US', legalLinks: { privacyUrl: null, termsUrl: TERMS_URL } },
			})),
	)

	expect((await fetchBoot()).legal).toEqual({ privacyUrl: '', termsUrl: TERMS_URL })
})

test('settles the interface on the locale the graph resolves', async () => {
	server.use(graphql.query('AppLocale', () => HttpResponse.json({ data: { locale: 'es-ES' } })))

	const settled = await startAppLocale()

	expect(settled.locale).toBe('es-ES')
	expect(displayLocale()).toBe('es-ES')
})

test('settles on the default when the graph cannot say', async () => {
	server.use(graphql.query('AppLocale', () => HttpResponse.error()))

	expect((await startAppLocale()).locale).toBe('en-US')
	expect(displayLocale()).toBe('en-US')
})

test('hands back the legal addresses from the one read that settled the locale', async () => {
	let reads = 0
	server.use(
		graphql.query('AppLocale', () => {
			reads += 1
			return HttpResponse.json({
				data: { locale: 'es-ES', legalLinks: { privacyUrl: PRIVACY_URL, termsUrl: TERMS_URL } },
			})
		}),
	)

	const settled = await startAppLocale()

	expect(settled.legal).toEqual({ privacyUrl: PRIVACY_URL, termsUrl: TERMS_URL })
	expect(reads).toBe(1)
})

test('names the domain AlphOne strings answer under', () => {
	expect(DOMAIN).toBe('alphone')
})

test('reads its own domain first and the brick domains last', () => {
	const domains = localeEntries().map((entry) => entry.domain)

	expect(domains[0]).toBe(DOMAIN)
	expect(domains.slice(-2)).toEqual(['gopherium-react-auth', LIST_CHROME_DOMAIN])
})

test('puts the list chrome in the language the graph resolves', async () => {
	server.use(graphql.query('AppLocale', () => HttpResponse.json({ data: { locale: 'es-ES' } })))

	await startAppLocale()

	expect(__('Add filter')).toBe('Añadir filtro')
})

test('reads every domain a compiled-in plugin declares, in registration order', () => {
	const domains = localeEntries().map((entry) => entry.domain)

	expect(domains.slice(1, -2)).toEqual(declaredEntries(plugins).map((entry) => entry.domain))
	expect(domains).toEqual(expect.arrayContaining(['alphone-fields', 'alphone-importer', 'alphone-whatsapp']))
})

test('reads each domain once, so no catalogue overwrites another', () => {
	const domains = localeEntries().map((entry) => entry.domain)

	expect(new Set(domains).size).toBe(domains.length)
})

test('skips a plugin declaring no domain of its own', () => {
	const declared = declaredEntries([{ id: 'bare', routes: () => [], nav: [] }])

	expect(declared).toEqual([])
})

test('renders core and plugin reasons from one merged map', () => {
	const merged = appErrorTemplates()

	expect(merged.contact_name_required).toBe('A contact needs a name.')
	expect(merged.field_name_taken).toBe('The field list just changed. Press Add field again.')
	expect(merged.import_not_found).toBe('That import no longer exists.')
	expect(merged.message_content_required).toBe('Write something to send.')
})

test('explains a write the server refused as sent from another site', () => {
	expect(appErrorTemplates().request_cross_origin).toBe(
		'This request came from a page on another site, so it was refused. Open the admin on this site and try again.',
	)
})

test('explains a refused cross-origin write in the language the graph resolves', async () => {
	server.use(graphql.query('AppLocale', () => HttpResponse.json({ data: { locale: 'es-ES' } })))

	await startAppLocale()

	expect(appErrorTemplates().request_cross_origin).toBe(
		'Esta petición vino de una página de otro sitio, así que se rechazó. ' +
			'Abre la administración en este sitio e inténtalo de nuevo.',
	)
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
